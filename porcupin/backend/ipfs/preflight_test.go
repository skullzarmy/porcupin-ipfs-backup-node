package ipfs

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbePort_FreePort(t *testing.T) {
	port := freePort(t)
	if err := probePort(port); err != nil {
		t.Fatalf("probePort(%d) on a free port returned %v, want nil", port, err)
	}
}

func TestProbePort_TCPBound(t *testing.T) {
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

func TestProbePort_UDPBound(t *testing.T) {
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

func TestPreflightCheck_FreshRepo(t *testing.T) {
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

func TestPreflightCheck_PortInUse(t *testing.T) {
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

func TestPreflightCheck_StaleLockTolerated(t *testing.T) {
	// A stale lock file (no live holder) should be tolerated at preflight —
	// the main Open() path handles removal. We just want preflight to NOT
	// fail in this case.
	tmpDir, err := os.MkdirTemp("", "ipfs-preflight-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "repo.lock"), []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	n := &Node{repoPath: tmpDir, swarmPort: freePort(t)}
	if err := n.preflightCheck(); err != nil {
		t.Fatalf("preflightCheck rejected a stale lock; should defer to Open(): %v", err)
	}
}
