package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCheckPriorCrashNoFile(t *testing.T) {
	dir := t.TempDir()
	got := CheckPriorCrash(dir)
	if got.Detected {
		t.Fatalf("Detected=true when no heartbeat file exists; got %+v", got)
	}
}

func TestCheckPriorCrashDeadPID(t *testing.T) {
	dir := t.TempDir()
	// pid 1 (init) exists; we want a definitely-dead pid. Use a very large pid
	// that is essentially guaranteed to be unused. On Linux pid_max defaults
	// to 32768 or 4194304; on macOS 99999. Using 2^31-1 is safe on all.
	deadPID := 0x7fffffff
	writeRawHeartbeat(t, dir, deadPID, time.Now().Add(-2*time.Minute))

	got := CheckPriorCrash(dir)
	if !got.Detected {
		t.Fatalf("expected Detected=true for dead pid %d, got %+v", deadPID, got)
	}
	if got.PID != deadPID {
		t.Errorf("PID = %d, want %d", got.PID, deadPID)
	}
	if got.LastSeen.IsZero() {
		t.Error("LastSeen should not be zero for a parseable marker")
	}
}

func TestCheckPriorCrashLivePID(t *testing.T) {
	dir := t.TempDir()
	// Our own pid is definitely alive. A heartbeat containing it should be
	// interpreted as "another instance running", NOT a crash.
	writeRawHeartbeat(t, dir, os.Getpid(), time.Now())

	got := CheckPriorCrash(dir)
	if got.Detected {
		t.Fatalf("Detected=true for live pid %d; should be treated as live instance; got %+v", os.Getpid(), got)
	}
}

func TestCheckPriorCrashUnparseable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, heartbeatFile), []byte("garbage"), 0644); err != nil {
		t.Fatal(err)
	}

	got := CheckPriorCrash(dir)
	if !got.Detected {
		t.Fatal("Detected=false for unparseable marker; should still surface as crash")
	}
	if got.PID != 0 {
		t.Errorf("PID = %d, want 0 when unparseable", got.PID)
	}
	if !got.LastSeen.IsZero() {
		t.Errorf("LastSeen = %v, want zero when unparseable", got.LastSeen)
	}
}

func TestStartHeartbeatWritesAndRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, heartbeatFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := StartHeartbeat(ctx, dir)

	// File should exist immediately after StartHeartbeat (initial write).
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("heartbeat file not written on start: %v", err)
	}

	// File should contain our pid.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var gotPID int
	var gotUnix int64
	if _, err := fmt.Sscanf(string(data), "%d %d", &gotPID, &gotUnix); err != nil {
		t.Fatalf("heartbeat file unparseable: %q", string(data))
	}
	if gotPID != os.Getpid() {
		t.Errorf("heartbeat pid = %d, want %d", gotPID, os.Getpid())
	}

	stop()

	// File should be removed after clean stop.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("heartbeat file still present after stop: err=%v", err)
	}
}

func TestStartHeartbeatTickerUpdatesFile(t *testing.T) {
	// Verify the ticker actually re-writes the file periodically — not just
	// the initial write. Without this, a broken ticker would still pass the
	// other tests.
	dir := t.TempDir()
	path := filepath.Join(dir, heartbeatFile)

	prev := heartbeatInterval
	heartbeatInterval = 20 * time.Millisecond
	defer func() { heartbeatInterval = prev }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := StartHeartbeat(ctx, dir)
	defer stop()

	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("initial heartbeat missing: %v", err)
	}
	// Wait several intervals; mtime should advance via the ticker path.
	time.Sleep(150 * time.Millisecond)
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("heartbeat removed unexpectedly: %v", err)
	}
	if !info2.ModTime().After(info1.ModTime()) {
		t.Fatalf("ticker did not re-write heartbeat: mtime unchanged at %v", info1.ModTime())
	}
}

func TestStartHeartbeatStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := StartHeartbeat(ctx, dir)

	// Second call must not panic (sync.Once guard).
	stop()
	stop()
	stop()
}

func TestStartHeartbeatContextCancelExitsGoroutine(t *testing.T) {
	// Verify that cancelling the context — without ever calling stop() —
	// terminates the background writer goroutine. We prove this by observing
	// that the heartbeat file stops being touched after cancellation.
	dir := t.TempDir()
	path := filepath.Join(dir, heartbeatFile)

	// Use a fast interval so we don't have to wait 30s for proof.
	prev := heartbeatInterval
	heartbeatInterval = 20 * time.Millisecond
	defer func() { heartbeatInterval = prev }()

	ctx, cancel := context.WithCancel(context.Background())
	_ = StartHeartbeat(ctx, dir)
	// Let the writer run for a few ticks.
	time.Sleep(80 * time.Millisecond)
	cancel()

	// Allow any in-flight write to flush, then snapshot mtime.
	time.Sleep(50 * time.Millisecond)
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("heartbeat file gone before cancel observation: %v", err)
	}
	mtime1 := info1.ModTime()

	// Wait several intervals. If the goroutine survived, mtime would advance.
	time.Sleep(200 * time.Millisecond)
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("heartbeat file gone after cancel: %v", err)
	}
	if !info2.ModTime().Equal(mtime1) {
		t.Fatalf("heartbeat file kept being written after ctx cancellation: mtime moved %v → %v",
			mtime1, info2.ModTime())
	}
}

// writeRawHeartbeat writes a heartbeat file with arbitrary pid + timestamp,
// bypassing the live writer. Used to simulate prior-process state.
func writeRawHeartbeat(t *testing.T, dir string, pid int, ts time.Time) {
	t.Helper()
	content := strconv.Itoa(pid) + " " + strconv.FormatInt(ts.Unix(), 10)
	if err := os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
