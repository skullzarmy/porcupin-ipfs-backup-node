package ipfs

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/kubo/config"
)

// freePort returns a port that is free on BOTH TCP and UDP, since Kubo and
// the preflight probe bind both. It verifies reusability with the same
// probePort the production code uses, retrying with a different ephemeral
// port if the probe fails (TIME_WAIT, concurrent process, etc.). Loops up
// to 20 times to absorb transient kernel races.
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 20; i++ {
		// Bind on all interfaces (":0") so the port the kernel hands us is
		// immediately re-bindable by code that listens on the same address
		// form, which is what the preflight probe and libp2p do.
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("could not allocate free TCP port: %v", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()

		if err := probePort(port); err != nil {
			continue
		}
		return port
	}
	t.Fatal("could not find a port free on both TCP and UDP after 20 attempts")
	return 0
}

func TestSanitizeDelegatedRouters(t *testing.T) {
	const router = "https://my-router.example/routing/v1"

	tests := []struct {
		name         string
		in           []string
		wantAccepted []string
		wantRejected []string
	}{
		{"nil", nil, nil, nil},
		{"auto only", []string{"auto"}, []string{"auto"}, nil},
		{"https url", []string{router}, []string{router}, nil},
		{"http url", []string{"http://r.example/routing/v1"}, []string{"http://r.example/routing/v1"}, nil},
		{"auto plus custom", []string{"auto", router}, []string{"auto", router}, nil},
		{"trims whitespace", []string{"  auto  "}, []string{"auto"}, nil},
		{"skips blanks", []string{"", "auto", "   "}, []string{"auto"}, nil},
		{"dedupes", []string{"auto", "auto", router, router}, []string{"auto", router}, nil},
		{"rejects non-url", []string{"not-a-url"}, nil, []string{"not-a-url"}},
		{"rejects bad scheme", []string{"ftp://x/routing/v1"}, nil, []string{"ftp://x/routing/v1"}},
		{"rejects missing host", []string{"https://"}, nil, []string{"https://"}},
		{"mixed valid and invalid", []string{"auto", "bogus", router}, []string{"auto", router}, []string{"bogus"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accepted, rejected := SanitizeDelegatedRouters(tt.in)
			if !equalStringSlices(accepted, tt.wantAccepted) {
				t.Errorf("accepted = %v, want %v", accepted, tt.wantAccepted)
			}
			if !equalStringSlices(rejected, tt.wantRejected) {
				t.Errorf("rejected = %v, want %v", rejected, tt.wantRejected)
			}
		})
	}
}

func TestApplyDelegatedRouters(t *testing.T) {
	const router = "https://my-router.example/routing/v1"

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil defaults to auto", nil, []string{"auto"}},
		{"auto preserved", []string{"auto"}, []string{"auto"}},
		{"custom added to auto", []string{"auto", router}, []string{"auto", router}},
		{"all invalid falls back to auto", []string{"bogus", "also-bad"}, []string{"auto"}},
		{"explicit empty disables (DHT only)", []string{}, nil},
		{"invalid dropped, valid kept", []string{"auto", "bogus"}, []string{"auto"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			applyDelegatedRouters(cfg, tt.in)
			if !equalStringSlices(cfg.Routing.DelegatedRouters, tt.want) {
				t.Errorf("DelegatedRouters = %v, want %v", cfg.Routing.DelegatedRouters, tt.want)
			}
		})
	}

	// Nil config must not panic.
	applyDelegatedRouters(nil, []string{"auto"})
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
