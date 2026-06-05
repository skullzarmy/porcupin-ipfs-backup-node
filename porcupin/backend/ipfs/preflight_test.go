package ipfs

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbePortFreePort(t *testing.T) {
	port := freePort(t)
	if err := probePort(port); err != nil {
		t.Fatalf("probePort(%d) on a free port returned %v, want nil", port, err)
	}
}

func TestProbePortTCPBound(t *testing.T) {
	// Hold a TCP listener so probePort sees the port as taken.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	err = probePort(port)
	if err == nil {
		t.Fatalf("probePort(%d) returned nil despite TCP being bound", port)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("tcp/%d", port)) {
		t.Errorf("error %q should identify tcp/<port>", err)
	}
}

func TestProbePortUDPBound(t *testing.T) {
	// Hold a UDP listener; TCP is free, UDP is not — probePort should still fail.
	udp, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	port := udp.LocalAddr().(*net.UDPAddr).Port

	err = probePort(port)
	if err == nil {
		t.Fatalf("probePort(%d) returned nil despite UDP being bound", port)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("udp/%d", port)) {
		t.Errorf("error %q should identify udp/<port>", err)
	}
}

func TestNodePreflightCheckFreshRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ipfs-preflight-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	n := &Node{repoPath: tmpDir, swarmPort: freePort(t)}
	if err := n.preflightCheck(); err != nil {
		t.Fatalf("preflightCheck on fresh repo + free port returned %v, want nil", err)
	}
}

func TestNodePreflightCheckPortInUse(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ipfs-preflight-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	n := &Node{repoPath: tmpDir, swarmPort: port}
	err = n.preflightCheck()
	if err == nil {
		t.Fatal("preflightCheck returned nil despite port conflict")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error %q should mention port already in use", err)
	}
}

func TestNodePreflightCheckUnheldLockTolerated(t *testing.T) {
	// preflight must NOT fail when a lock FILE exists but no process holds
	// it (stale lock). The actual removal is delegated to the main Open()
	// flow in Start(), via removeStaleLock(). Here we verify preflight
	// defers — i.e. doesn't itself reject startup.
	//
	// Note: this only exercises the "file present but go-fs-lock reports
	// not held" branch; that's what preflight branches on. The deeper
	// stale-lock-recovery behavior (removeStaleLock) is exercised in
	// Start() integration tests.
	tmpDir, err := os.MkdirTemp("", "ipfs-preflight-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "repo.lock"), []byte("unheld"), 0600); err != nil {
		t.Fatal(err)
	}

	n := &Node{repoPath: tmpDir, swarmPort: freePort(t)}
	if err := n.preflightCheck(); err != nil {
		t.Fatalf("preflightCheck rejected an unheld lock; should defer to Open(): %v", err)
	}
}
