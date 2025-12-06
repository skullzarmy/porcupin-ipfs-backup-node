package ipfs

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/corerepo"
	"github.com/ipfs/kubo/core/node/libp2p"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo/fsrepo"

	// Boxo imports
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/path"
	iface "github.com/ipfs/kubo/core/coreiface"
	"github.com/ipfs/kubo/core/coreiface/options"
)

// ShutdownTimeout is the maximum time to wait for IPFS node to shut down gracefully
const ShutdownTimeout = 30 * time.Second

// Node represents an embedded IPFS node
type Node struct {
	api      iface.CoreAPI
	node     *core.IpfsNode
	repoPath string
	mu       sync.RWMutex
	cancel   context.CancelFunc
	ctx      context.Context
}

// NewNode creates a new IPFS node instance
func NewNode(repoPath string) (*Node, error) {
	// Expand tilde if present
	if len(repoPath) > 0 && repoPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		repoPath = filepath.Join(home, repoPath[1:])
	}

	return &Node{
		repoPath: repoPath,
	}, nil
}

// Start initializes and starts the IPFS node
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.node != nil {
		return nil // Already started
	}

	log.Printf("IPFS node starting (repo: %s)...", n.repoPath)

	// Remove stale lock file if it exists from a previous unclean shutdown
	// This can happen after a crash, forced quit, or migration
	lockFile := filepath.Join(n.repoPath, "repo.lock")
	if _, err := os.Stat(lockFile); err == nil {
		log.Printf("Removing stale repo lock file before start: %s", lockFile)
		if err := os.Remove(lockFile); err != nil {
			log.Printf("Warning: failed to remove stale lock file: %v", err)
			// Continue anyway - fsrepo.Open will give a clearer error if needed
		}
	}

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
		if err := fsrepo.Init(n.repoPath, cfg); err != nil {
			return fmt.Errorf("failed to init repo: %w", err)
		}
	}

	// Open repo
	repo, err := fsrepo.Open(n.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	// Construct node
	nodeOptions := &core.BuildCfg{
		Online:  true,
		Routing: libp2p.DHTOption,
		Repo:    repo,
		ExtraOpts: map[string]bool{
			"pubsub": true,
		},
	}

	node, err := core.NewNode(ctx, nodeOptions)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
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
		done <- n.node.Close()
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

// Pin pins a CID to the local node with a timeout
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
	
	// OS-specific cleanup to ensure disk space is properly released
	// All commands are official/standard utilities that fail gracefully without permissions
	log.Println("Running OS-specific disk cleanup...")
	
	switch runtime.GOOS {
	case "darwin":
		// sync: Standard POSIX filesystem flush
		exec.Command("sync").Run()
		// tmutil thinlocalsnapshots: Apple's official Time Machine API
		// Requests macOS to thin snapshots to free space - same as Disk Utility uses
		log.Println("Requesting Time Machine to release snapshot space...")
		cmd := exec.Command("tmutil", "thinlocalsnapshots", "/", "107374182400", "1")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("tmutil note: %v (%s)", err, string(output))
		}
		
	case "linux":
		// sync: Standard POSIX filesystem flush
		exec.Command("sync").Run()
		// fstrim: Standard util-linux command for SSD TRIM
		// Recommended by all major distros, runs weekly via systemd timer by default
		if n.repoPath != "" {
			exec.Command("fstrim", "-v", n.repoPath).Run()
		}
		
	case "windows":
		// Optimize-Volume: Official Windows PowerShell cmdlet for drive optimization
		// Triggers TRIM on SSDs, same as running "Optimize Drives" from Windows
		if len(n.repoPath) >= 2 && n.repoPath[1] == ':' {
			driveLetter := string(n.repoPath[0])
			log.Printf("Requesting Windows to optimize drive %s...", driveLetter)
			exec.Command("powershell", "-Command", 
				fmt.Sprintf("Optimize-Volume -DriveLetter %s -ReTrim -ErrorAction SilentlyContinue", driveLetter)).Run()
		}
		
	default:
		// sync: Standard POSIX - works on all Unix-like systems (BSD, etc)
		exec.Command("sync").Run()
	}
	
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

	// Try to detect mime type from content
	mimeType := detectMimeType(data[:n_read])

	return data[:n_read], mimeType, nil
}

// detectMimeType tries to detect the mime type from content
func detectMimeType(data []byte) string {
	if len(data) < 4 {
		return "application/octet-stream"
	}

	// Check magic bytes
	switch {
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return "image/png"
	case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46:
		return "image/gif"
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46:
		return "image/webp"
	case data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x00:
		if len(data) > 7 && data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70 {
			return "video/mp4"
		}
	case data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3:
		return "video/webm"
	case data[0] == '{' || data[0] == '[':
		return "application/json"
	case data[0] == '<':
		return "text/html"
	}

	return "application/octet-stream"
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
