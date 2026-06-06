//go:build windows

package logging

import (
	"golang.org/x/sys/windows"
)

// processAlive on Windows opens the process with the minimum-privilege
// PROCESS_QUERY_LIMITED_INFORMATION right and inspects its exit code. A live
// process reports STILL_ACTIVE (259); anything else means the pid is either
// gone or has been recycled into a finished process.
//
// os.FindProcess on Windows always succeeds even for dead pids, and there is
// no equivalent of signal 0, so this is the canonical existence check.
//
// LIMITATION: same pid-recycling caveat as the Unix variant — a recycled pid
// belonging to an unrelated live process will report "alive" and the crash
// detector will miss the prior crash.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// ERROR_INVALID_PARAMETER / ERROR_ACCESS_DENIED both effectively mean
		// "no such live process we can confirm" — treat as dead so the crash
		// detection path can run.
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return exitCode == stillActive
}
