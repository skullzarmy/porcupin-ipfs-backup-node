package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// heartbeatFile is the filename written under dataDir to indicate the process
// is alive. It contains the current pid and the timestamp of the last write.
const heartbeatFile = "last-alive"

// HeartbeatInterval is the default cadence at which the heartbeat file is
// updated. Tests override the package-level heartbeatInterval below to
// shorten this without exposing it as a public API.
const HeartbeatInterval = 30 * time.Second

// heartbeatInterval is the value actually used by StartHeartbeat. It defaults
// to HeartbeatInterval but tests in this package may override it to avoid
// 30-second waits.
var heartbeatInterval = HeartbeatInterval

// PriorCrashInfo describes evidence of a previous process that did not shut
// down cleanly. Returned by CheckPriorCrash.
type PriorCrashInfo struct {
	Detected bool      // true if the previous run died without removing the heartbeat
	PID      int       // pid recorded in the stale heartbeat (0 if unknown)
	LastSeen time.Time // timestamp recorded in the stale heartbeat (zero if unknown)
}

// CheckPriorCrash inspects the heartbeat file written by a previous process.
// If the file exists and the recorded pid is no longer running, it concludes
// the previous run died ungracefully (SIGKILL, OOM, power loss, etc.) and
// returns Detected=true. If the recorded pid is still alive, another instance
// is presumed running and Detected=false is returned.
//
// Call this BEFORE StartHeartbeat so we read the previous run's marker, not
// our own.
func CheckPriorCrash(dataDir string) PriorCrashInfo {
	path := filepath.Join(dataDir, heartbeatFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return PriorCrashInfo{}
	}

	var pid int
	var unix int64
	// Format: "<pid> <unix_seconds>"
	if _, err := fmt.Sscanf(string(data), "%d %d", &pid, &unix); err != nil {
		// Unparseable — treat as a stale marker but with no useful detail
		return PriorCrashInfo{Detected: true}
	}

	// Process still alive? Then it's not a crash marker — another instance is
	// running. Don't claim a crash.
	if pid > 0 && processAlive(pid) {
		return PriorCrashInfo{}
	}

	return PriorCrashInfo{
		Detected: true,
		PID:      pid,
		LastSeen: time.Unix(unix, 0),
	}
}

// StartHeartbeat begins writing the heartbeat file at HeartbeatInterval.
// Returns a stop function that should be called on clean shutdown to remove
// the heartbeat file (signaling "this exit was intentional"). The stop
// function is idempotent — calling it more than once is safe.
func StartHeartbeat(ctx context.Context, dataDir string) func() {
	path := filepath.Join(dataDir, heartbeatFile)
	if err := writeHeartbeat(path); err != nil {
		slog.Warn("could not write initial heartbeat", "error", err)
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := writeHeartbeat(path); err != nil {
					slog.Warn("could not update heartbeat", "error", err)
				}
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(stopCh)
			<-doneCh
			// Clean shutdown — remove the marker so the next launch doesn't
			// report a phantom crash.
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				slog.Warn("could not remove heartbeat on clean shutdown", "error", err)
			}
		})
	}
}

func writeHeartbeat(path string) error {
	content := strconv.Itoa(os.Getpid()) + " " + strconv.FormatInt(time.Now().Unix(), 10)
	// 0600: pid + timestamp aren't sensitive, but other ~/.porcupin files
	// (crash reports, db) are user-private, so keep the heartbeat consistent.
	return os.WriteFile(path, []byte(content), 0600)
}
