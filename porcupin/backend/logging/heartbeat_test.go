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

func TestCheckPriorCrash_NoFile(t *testing.T) {
	dir := t.TempDir()
	got := CheckPriorCrash(dir)
	if got.Detected {
		t.Fatalf("Detected=true when no heartbeat file exists; got %+v", got)
	}
}

func TestCheckPriorCrash_DeadPID(t *testing.T) {
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

func TestCheckPriorCrash_LivePID(t *testing.T) {
	dir := t.TempDir()
	// Our own pid is definitely alive. A heartbeat containing it should be
	// interpreted as "another instance running", NOT a crash.
	writeRawHeartbeat(t, dir, os.Getpid(), time.Now())

	got := CheckPriorCrash(dir)
	if got.Detected {
		t.Fatalf("Detected=true for live pid %d; should be treated as live instance; got %+v", os.Getpid(), got)
	}
}

func TestCheckPriorCrash_Unparseable(t *testing.T) {
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

func TestStartHeartbeat_WritesAndRemovesFile(t *testing.T) {
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

func TestStartHeartbeat_StopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := StartHeartbeat(ctx, dir)

	// Second call must not panic (sync.Once guard).
	stop()
	stop()
	stop()
}

func TestStartHeartbeat_ContextCancelStopsGoroutine(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	stop := StartHeartbeat(ctx, dir)
	defer stop()

	cancel()
	// Give the goroutine a moment to exit via ctx.Done. Stop should then
	// return promptly because doneCh is already closed.
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return after context cancellation")
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
