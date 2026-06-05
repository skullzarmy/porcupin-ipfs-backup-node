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
