package ipfs

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// freePort asks the kernel for an ephemeral TCP port and returns it.
// Tests need this to avoid the well-known 4001 swarm port which is often
// taken on developer machines by other IPFS daemons.
func freePort(t *testing.T) int {
	t.Helper()
	// Bind on all interfaces (":0") so the port the kernel hands us is
	// immediately re-bindable by code that listens on the same address form,
	// which is what the preflight probe and libp2p do. Binding 127.0.0.1 here
	// and then 0.0.0.0 there can hit transient EADDRINUSE on some kernels.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("could not allocate free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestNodePinAndVerify(t *testing.T) {
	// Create a temporary directory for the test IPFS repo
	tmpDir, err := os.MkdirTemp("", "ipfs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := filepath.Join(tmpDir, "ipfs")

	// Create and start node on a free port (4001 is often taken on dev machines)
	node, err := NewNode(repoPath, freePort(t))
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer node.Stop()

	// Add some test content
	testContent := []byte("Hello, IPFS! This is a test content for verification.")
	cid, err := node.Add(ctx, bytes.NewReader(testContent))
	if err != nil {
		t.Fatalf("Failed to add content: %v", err)
	}
	t.Logf("Added content with CID: %s", cid)

	// Test IsPinned
	pinned, err := node.IsPinned(ctx, cid)
	if err != nil {
		t.Fatalf("Failed to check pin status: %v", err)
	}
	if !pinned {
		t.Error("Content should be pinned after Add")
	}

	// Test Verify
	result := node.Verify(ctx, cid, 30*time.Second)
	if !result.IsPinned {
		t.Error("Verify should report content as pinned")
	}
	if !result.IsAvailable {
		t.Error("Verify should report content as available")
	}
	if result.Size != int64(len(testContent)) {
		t.Errorf("Size mismatch: expected %d, got %d", len(testContent), result.Size)
	}
	if result.Error != "" {
		t.Errorf("Unexpected error in verify result: %s", result.Error)
	}

	// Test Cat
	data, mimeType, err := node.Cat(ctx, cid, 1024)
	if err != nil {
		t.Fatalf("Failed to cat content: %v", err)
	}
	if !bytes.Equal(data, testContent) {
		t.Error("Cat returned different content than what was added")
	}
	t.Logf("Cat returned %d bytes with mime type: %s", len(data), mimeType)

	// Test Unpin and verify again
	if err := node.Unpin(ctx, cid); err != nil {
		t.Fatalf("Failed to unpin: %v", err)
	}

	pinned, err = node.IsPinned(ctx, cid)
	if err != nil {
		t.Fatalf("Failed to check pin status after unpin: %v", err)
	}
	if pinned {
		t.Error("Content should not be pinned after Unpin")
	}
}
