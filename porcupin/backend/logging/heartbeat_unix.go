//go:build !windows

package logging

import (
	"errors"
	"os"
	"syscall"
)

// processAlive uses signal 0 as a portable existence probe on Unix.
//
//   - nil error  → process exists and we are allowed to signal it.
//   - EPERM      → process exists but is owned by another user; still alive.
//   - ESRCH      → no such process; dead.
//   - other      → conservatively treat as dead so we surface the crash
//     rather than swallow it on a transient permission/lookup error.
//
// LIMITATION: PID recycling can produce a false "alive" verdict — if the
// crashed Porcupin process's pid was reassigned by the OS to an unrelated
// live process before we check, we'll think Porcupin is still running and
// suppress the crash-detection event. Mitigating this would require
// recording the process start time alongside the pid (per-OS work); for now
// we accept the false negative in exchange for a simple, dependency-free
// check.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
