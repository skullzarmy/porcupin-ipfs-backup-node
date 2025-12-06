//go:build windows
// +build windows

package main

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// getFreeDiskSpaceGB returns the free disk space in GB (Windows implementation)
func getFreeDiskSpaceGB() float64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable uint64
	var totalNumberOfBytes uint64
	var totalNumberOfFreeBytes uint64

	path, _ := syscall.UTF16PtrFromString("C:\\")
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)

	if ret != 0 {
		return float64(freeBytesAvailable) / (1024 * 1024 * 1024)
	}
	return 0
}

// Cached disk size to avoid running expensive walk multiple times per second
var (
	diskSizeCache     = make(map[string]int64)
	diskSizeCacheTime = make(map[string]time.Time)
	diskSizeCacheMu   sync.RWMutex
	diskSizeCacheTTL  = 30 * time.Second
)

// getDirSizeBytes returns the actual disk usage of a directory in bytes (Windows implementation)
// Uses robocopy with /L (list-only) flag which is significantly faster than filepath.Walk
// for large directories with many files. Robocopy is built into Windows since Vista.
// Results are cached for 30 seconds to avoid expensive repeated calls.
func getDirSizeBytes(path string) (int64, error) {
	diskSizeCacheMu.RLock()
	if cachedSize, ok := diskSizeCache[path]; ok {
		if time.Since(diskSizeCacheTime[path]) < diskSizeCacheTTL {
			diskSizeCacheMu.RUnlock()
			return cachedSize, nil
		}
	}
	diskSizeCacheMu.RUnlock()

	// Use robocopy in list-only mode to get directory size
	// This is much faster than filepath.Walk for large directories
	// /L = List only, /S = include subdirectories, /NFL /NDL /NJH /NJS = minimize output
	// /BYTES = show sizes in bytes
	// Output format includes: "Bytes : 1234567890"
	cmd := exec.Command("robocopy", path, "NUL", "/L", "/S", "/NFL", "/NDL", "/NJH", "/BYTES")
	
	// robocopy returns non-zero exit codes that don't mean failure:
	// 0-7 = various success states, 8+ = actual errors
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's just a non-fatal robocopy exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() < 8 {
				err = nil // Not a real error
			}
		}
	}
	
	if err != nil {
		log.Printf("robocopy failed, falling back to PowerShell: %v", err)
		return getDirSizePowerShell(path)
	}

	// Parse output for "Bytes : <number>"
	var size int64
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Bytes :") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				size, _ = strconv.ParseInt(parts[2], 10, 64)
				break
			}
		}
	}

	diskSizeCacheMu.Lock()
	diskSizeCache[path] = size
	diskSizeCacheTime[path] = time.Now()
	diskSizeCacheMu.Unlock()

	return size, nil
}

// getDirSizePowerShell is a fallback method using PowerShell
func getDirSizePowerShell(path string) (int64, error) {
	// Use PowerShell to get directory size efficiently
	// (Get-ChildItem -Path 'path' -Recurse -Force -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum
	psCmd := `(Get-ChildItem -Path '` + strings.ReplaceAll(path, "'", "''") + `' -Recurse -Force -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	
	output, err := cmd.Output()
	if err != nil {
		log.Printf("PowerShell fallback also failed: %v", err)
		return 0, err
	}

	sizeStr := strings.TrimSpace(string(output))
	size, _ := strconv.ParseInt(sizeStr, 10, 64)

	diskSizeCacheMu.Lock()
	diskSizeCache[path] = size
	diskSizeCacheTime[path] = time.Now()
	diskSizeCacheMu.Unlock()

	return size, nil
}
