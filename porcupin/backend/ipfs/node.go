package ipfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/corerepo"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo"
	"github.com/ipfs/kubo/repo/fsrepo"

	// Boxo imports
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/path"
	iface "github.com/ipfs/kubo/core/coreiface"
	"github.com/ipfs/kubo/core/coreiface/options"
)

// ShutdownTimeout is the maximum time to wait for IPFS node to shut down gracefully
const ShutdownTimeout = 30 * time.Second

// DefaultSwarmPort is the default port for IPFS swarm connections
const DefaultSwarmPort = 4001

// Node represents an embedded IPFS node
type Node struct {
	api       iface.CoreAPI
	node      *core.IpfsNode
	repoPath  string
	swarmPort int
	mu        sync.RWMutex
	cancel    context.CancelFunc
	ctx       context.Context
}

// NewNode creates a new IPFS node instance
// swarmPort specifies the port for p2p connections (0 uses default 4001)
func NewNode(repoPath string, swarmPort int) (*Node, error) {
	// Expand tilde if present
	if len(repoPath) > 0 && repoPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		repoPath = filepath.Join(home, repoPath[1:])
	}

	// Use default port if not specified
	if swarmPort <= 0 {
		swarmPort = DefaultSwarmPort
	}

	return &Node{
		repoPath:  repoPath,
		swarmPort: swarmPort,
	}, nil
}

// Start initializes and starts the IPFS node
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.node != nil {
		return nil // Already started
	}

	log.Printf("IPFS node starting (repo: %s, swarm port: %d)...", n.repoPath, n.swarmPort)

	// Setup plugins
	if err := setupPlugins(""); err != nil {
		return fmt.Errorf("failed to setup plugins: %w", err)
	}

	// Initialize repo if not exists
	if !fsrepo.IsInitialized(n.repoPath) {
		log.Printf("Initializing new IPFS repo at %s", n.repoPath)
		cfg, err := config.Init(io.Discard, 2048)
		if err != nil {
			return fmt.Errorf("failed to init config: %w", err)
		}
		// Configure swarm addresses with custom port
		n.configureSwarmAddresses(cfg)

		// Apply profile-specific configuration (e.g., low power settings)
		applyProfileConfig(cfg)

		if err := fsrepo.Init(n.repoPath, cfg); err != nil {
			return fmt.Errorf("failed to init repo: %w", err)
		}
	}

	// Open repo. If locked, surface a clear user-facing error rather than automatically
	// removing the lock file. OS file locks (flock) are released by the kernel when a
	// process exits — including crashes and SIGKILL — so a stale lock from a previous
	// run is self-healing. Automatic removal risks corrupting a live repo.
	repo, err := fsrepo.Open(n.repoPath)
	if err != nil {
		if strings.Contains(err.Error(), "lock") {
			lockFile := filepath.Join(n.repoPath, "repo.lock")
			return fmt.Errorf("IPFS repository is locked by another process. If Porcupin is not already running, delete %s and try again", lockFile)
		}
		return fmt.Errorf("failed to open repo: %w", err)
	}

	// Update swarm port in existing repo config if it differs
	if err := n.updateRepoSwarmPort(repo); err != nil {
		repo.Close()
		return fmt.Errorf("failed to update swarm port: %w", err)
	}

	// Construct node
	nodeOptions := &core.BuildCfg{
		Online:  true,
		Routing: getRoutingOption(),
		Repo:    repo,
		ExtraOpts: map[string]bool{
			"pubsub": true,
		},
	}

	var node *core.IpfsNode
	for attempt := 1; attempt <= 3; attempt++ {
		node, err = core.NewNode(ctx, nodeOptions)
		if err == nil {
			break
		}
		if !isPortConflictError(err) {
			repo.Close()
			return fmt.Errorf("failed to create node: %w", err)
		}
		if attempt < 3 {
			log.Printf("Port %d in use (attempt %d/3), retrying in 2s...", n.swarmPort, attempt)
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		repo.Close()
		return fmt.Errorf("failed to create node: %w", err)
	}

	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		node.Close()
		return fmt.Errorf("failed to create core api: %w", err)
	}

	n.node = node
	n.api = api
	n.ctx, n.cancel = context.WithCancel(ctx)
	
	log.Printf("IPFS node started successfully")

	return nil
}

// Stop shuts down the IPFS node with a timeout to prevent hanging
func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.node == nil {
		log.Println("IPFS node already stopped")
		return nil
	}

	repoPath := n.repoPath
	log.Printf("IPFS node shutdown starting (repo: %s)...", repoPath)

	// Cancel the node's context first to signal all operations to stop
	if n.cancel != nil {
		n.cancel()
	}

	// Close the node with a timeout - node.Close() can hang indefinitely
	// when there are active libp2p connections or DHT operations
	done := make(chan error, 1)
	go func() {
		closeErr := n.node.Close()
		if closeErr != nil {
			log.Printf("IPFS deferred close completed with error: %v", closeErr)
		} else {
			log.Println("IPFS deferred close completed successfully")
		}
		done <- closeErr
	}()

	var closeErr error
	select {
	case err := <-done:
		closeErr = err
		if err != nil {
			log.Printf("IPFS node closed with error: %v", err)
		} else {
			log.Println("IPFS node shutdown complete")
		}
	case <-time.After(ShutdownTimeout):
		log.Printf("IPFS node shutdown timed out after %v, forcing closure", ShutdownTimeout)
		// Node didn't close in time - this can happen when libp2p connections are stuck
		// We need to proceed anyway, so we'll clean up the lock file manually
	}

	// Clear our references regardless of how we exited
	n.node = nil
	n.api = nil

	// Remove the repo lock file if it exists - this is necessary when:
	// 1. The node didn't shut down cleanly (timeout)
	// 2. We need to allow migration to proceed
	// 3. We need to allow a new node to start at a different location
	lockFile := filepath.Join(repoPath, "repo.lock")
	if _, err := os.Stat(lockFile); err == nil {
		log.Printf("Removing stale repo lock file: %s", lockFile)
		if err := os.Remove(lockFile); err != nil {
			log.Printf("Warning: failed to remove lock file: %v", err)
		}
	}

	return closeErr
}

// isPortConflictError returns true if the error indicates the swarm port is already bound.
// Checks via errors.As for net.OpError / syscall.EADDRINUSE, with a string-match fallback
// for cases where the error is wrapped inside IPFS/multiaddr layers.
func isPortConflictError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return true
		}
	}
	// Fallback: covers errors wrapped so deeply that errors.As can't reach the OpError
	return strings.Contains(err.Error(), "address already in use")
}

// configureSwarmAddresses sets up the swarm listen addresses with the configured port
func (n *Node) configureSwarmAddresses(cfg *config.Config) {
	port := n.swarmPort
	cfg.Addresses.Swarm = []string{
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
		fmt.Sprintf("/ip6/::/tcp/%d", port),
		fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port),
		fmt.Sprintf("/ip6/::/udp/%d/quic-v1", port),
		fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1/webtransport", port),
		fmt.Sprintf("/ip6/::/udp/%d/quic-v1/webtransport", port),
	}
	log.Printf("Configured IPFS swarm addresses for port %d", port)
}

// updateRepoSwarmPort updates the swarm port in an existing repo if it differs from configured
func (n *Node) updateRepoSwarmPort(repo repo.Repo) error {
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("failed to get repo config: %w", err)
	}

	// Check if port needs updating by examining current addresses
	needsUpdate := true
	expectedTCP := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", n.swarmPort)
	for _, addr := range cfg.Addresses.Swarm {
		if addr == expectedTCP {
			needsUpdate = false
			break
		}
	}

	if needsUpdate {
		log.Printf("Updating IPFS swarm port from existing config to %d", n.swarmPort)
		n.configureSwarmAddresses(cfg)
		if err := repo.SetConfig(cfg); err != nil {
			return fmt.Errorf("failed to save updated config: %w", err)
		}
	}

	return nil
}

// GetSwarmPort returns the configured swarm port
func (n *Node) GetSwarmPort() int {
	return n.swarmPort
}

// NodeHealthResult holds the result of an IPFS node health check.
type NodeHealthResult struct {
	IsOnline  bool      `json:"is_online"`
	PeerCount int       `json:"peer_count"`
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

// Health queries the number of connected swarm peers and returns an IsOnline result.
// Uses a 10-second timeout and never mutates node state.
func (n *Node) Health(ctx context.Context) NodeHealthResult {
	// Copy api ref under lock and release immediately — Swarm().Peers() can block
	// for up to 10 seconds and holding RLock that long would delay Stop()/Start().
	n.mu.RLock()
	api := n.api
	n.mu.RUnlock()

	if api == nil {
		return NodeHealthResult{IsOnline: false, Message: "Node not started", CheckedAt: time.Now()}
	}

	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	peers, err := api.Swarm().Peers(tctx)
	if err != nil {
		return NodeHealthResult{IsOnline: false, Message: "Health check failed: " + err.Error(), CheckedAt: time.Now()}
	}
	if len(peers) == 0 {
		return NodeHealthResult{IsOnline: false, PeerCount: 0, Message: "Running but no peers connected", CheckedAt: time.Now()}
	}
	return NodeHealthResult{IsOnline: true, PeerCount: len(peers), Message: "Connected", CheckedAt: time.Now()}
}
func (n *Node) Pin(ctx context.Context, cidStr string, timeout time.Duration) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return fmt.Errorf("node not started")
	}

	// Ensure CID has /ipfs/ prefix
	if len(cidStr) > 0 && cidStr[0] != '/' {
		cidStr = "/ipfs/" + cidStr
	}

	p, err := path.NewPath(cidStr)
	if err != nil {
		return fmt.Errorf("invalid cid: %w", err)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Pin the content (recursive by default)
	if err := n.api.Pin().Add(ctx, p, options.Pin.Recursive(true)); err != nil {
		return fmt.Errorf("failed to pin: %w", err)
	}

	return nil
}

// Add adds data to the IPFS node and returns the CID
func (n *Node) Add(ctx context.Context, r io.Reader) (string, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return "", fmt.Errorf("node not started")
	}

	file := files.NewReaderFile(r)
	path, err := n.api.Unixfs().Add(ctx, file, options.Unixfs.Pin(true, ""))
	if err != nil {
		return "", fmt.Errorf("failed to add file: %w", err)
	}

	return path.RootCid().String(), nil
}

// Unpin removes a pin from the local node
func (n *Node) Unpin(ctx context.Context, cidStr string) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return fmt.Errorf("node not started")
	}

	// Ensure CID has /ipfs/ prefix
	if len(cidStr) > 0 && cidStr[0] != '/' {
		cidStr = "/ipfs/" + cidStr
	}

	p, err := path.NewPath(cidStr)
	if err != nil {
		return fmt.Errorf("invalid cid: %w", err)
	}

	// Unpin the content
	if err := n.api.Pin().Rm(ctx, p, options.Pin.RmRecursive(true)); err != nil {
		return fmt.Errorf("failed to unpin: %w", err)
	}

	return nil
}

// Stat returns the cumulative size of a CID (after pinning)
func (n *Node) Stat(ctx context.Context, cidStr string) (int64, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return 0, fmt.Errorf("node not started")
	}

	// Ensure CID has /ipfs/ prefix
	if len(cidStr) > 0 && cidStr[0] != '/' {
		cidStr = "/ipfs/" + cidStr
	}

	p, err := path.NewPath(cidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid cid: %w", err)
	}

	// Use Block().Stat to get size information
	// This returns size of the block, for recursive size we need to walk the DAG
	// For now, get the file and calculate size
	node, err := n.api.Unixfs().Get(ctx, p)
	if err != nil {
		return 0, fmt.Errorf("failed to get: %w", err)
	}
	defer node.Close()

	// Get size from the node
	size, err := node.Size()
	if err != nil {
		return 0, fmt.Errorf("failed to get size: %w", err)
	}

	return size, nil
}

// GetRepoPath returns the path to the IPFS repository
func (n *Node) GetRepoPath() string {
	return n.repoPath
}

// ProgressCallback is called during long operations to report progress
type ProgressCallback func(total, current int)

// UnpinAll removes all recursive pins from the node
func (n *Node) UnpinAll(ctx context.Context, progress ProgressCallback) (int, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return 0, fmt.Errorf("node not started")
	}

	// Create a channel for Pin().Ls to write pins to
	pinChan := make(chan iface.Pin)
	
	// Collect all CIDs first, then unpin them
	// This avoids issues with modifying pins while iterating
	var cids []string
	
	// Start listing pins in a goroutine - Ls closes the channel when done
	errChan := make(chan error, 1)
	go func() {
		errChan <- n.api.Pin().Ls(ctx, pinChan, options.Pin.Ls.Recursive())
	}()

	// Collect all CIDs from the channel
	for pin := range pinChan {
		cids = append(cids, pin.Path().RootCid().String())
	}

	// Check if Ls had an error
	if err := <-errChan; err != nil {
		log.Printf("Error listing pins: %v", err)
		return 0, fmt.Errorf("failed to list pins: %w", err)
	}

	total := len(cids)
	log.Printf("Found %d pins to remove", total)
	
	// Report initial progress
	if progress != nil {
		progress(total, 0)
	}

	// Now unpin each CID
	count := 0
	for _, cidStr := range cids {
		p, err := path.NewPath("/ipfs/" + cidStr)
		if err != nil {
			log.Printf("Invalid pin path: %v", err)
			continue
		}
		
		if err := n.api.Pin().Rm(ctx, p, options.Pin.RmRecursive(true)); err != nil {
			log.Printf("Failed to unpin %s: %v", cidStr, err)
			continue
		}
		count++
		
		// Report progress
		if progress != nil {
			progress(total, count)
		}
		
		if count%100 == 0 {
			log.Printf("Unpinned %d/%d items...", count, total)
		}
	}

	log.Printf("Unpinned %d items", count)
	return count, nil
}

// GarbageCollect runs IPFS garbage collection to free disk space
func (n *Node) GarbageCollect(ctx context.Context) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.node == nil {
		return fmt.Errorf("node not started")
	}

	log.Println("Starting IPFS garbage collection...")
	
	// Use corerepo.GarbageCollect which takes (node, ctx)
	if err := corerepo.GarbageCollect(n.node, ctx); err != nil {
		return fmt.Errorf("garbage collection failed: %w", err)
	}
	
	// OS-specific cleanup has been removed for safety.
	// We rely solely on IPFS internal repo garbage collection.
	
	log.Println("IPFS garbage collection complete")
	return nil
}

// VerifyResult represents the result of verifying a pinned asset
type VerifyResult struct {
	CID         string `json:"cid"`
	IsPinned    bool   `json:"is_pinned"`
	IsAvailable bool   `json:"is_available"`
	Size        int64  `json:"size"`
	Error       string `json:"error,omitempty"`
}

// IsPinned checks if a CID is pinned locally
func (n *Node) IsPinned(ctx context.Context, cidStr string) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return false, fmt.Errorf("node not started")
	}

	// Ensure CID has /ipfs/ prefix
	if len(cidStr) > 0 && cidStr[0] != '/' {
		cidStr = "/ipfs/" + cidStr
	}

	p, err := path.NewPath(cidStr)
	if err != nil {
		return false, fmt.Errorf("invalid cid: %w", err)
	}

	// Check if pinned
	_, pinned, err := n.api.Pin().IsPinned(ctx, p)
	if err != nil {
		return false, err
	}

	return pinned, nil
}

// Verify checks if a CID is pinned and can be retrieved
func (n *Node) Verify(ctx context.Context, cidStr string, timeout time.Duration) VerifyResult {
	result := VerifyResult{CID: cidStr}

	// Check if pinned
	pinned, err := n.IsPinned(ctx, cidStr)
	if err != nil {
		result.Error = fmt.Sprintf("pin check failed: %v", err)
		return result
	}
	result.IsPinned = pinned

	// Try to get the content to verify it's available
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	size, err := n.Stat(ctx, cidStr)
	if err != nil {
		result.Error = fmt.Sprintf("stat failed: %v", err)
		return result
	}

	result.IsAvailable = true
	result.Size = size
	return result
}

// Cat retrieves the content of a CID (for preview/testing)
func (n *Node) Cat(ctx context.Context, cidStr string, maxBytes int64) ([]byte, string, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.api == nil {
		return nil, "", fmt.Errorf("node not started")
	}

	// Ensure CID has /ipfs/ prefix
	if len(cidStr) > 0 && cidStr[0] != '/' {
		cidStr = "/ipfs/" + cidStr
	}

	p, err := path.NewPath(cidStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid cid: %w", err)
	}

	node, err := n.api.Unixfs().Get(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get: %w", err)
	}
	defer node.Close()

	// Get as file
	file, ok := node.(files.File)
	if !ok {
		return nil, "", fmt.Errorf("not a file")
	}

	// Read up to maxBytes
	data := make([]byte, maxBytes)
	n_read, err := file.Read(data)
	if err != nil && err != io.EOF {
		return nil, "", fmt.Errorf("failed to read: %w", err)
	}

	// Detect mime type using standard library
	mimeType := http.DetectContentType(data[:n_read])

	return data[:n_read], mimeType, nil
}


// Helper to setup plugins (required for Kubo)
func setupPlugins(externalPluginsPath string) error {
	plugins, err := loader.NewPluginLoader(filepath.Join(externalPluginsPath, "plugins"))
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error injecting plugins: %s", err)
	}

	return nil
}
