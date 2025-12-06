//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// getFreeDiskSpaceGB returns the free disk space in GB (Unix implementation)
func getFreeDiskSpaceGB() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		return float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	}
	return 0
}

// Cached disk size to avoid running du multiple times per second
var (
	diskSizeCache     = make(map[string]int64)
	diskSizeCacheTime = make(map[string]time.Time)
	diskSizeCacheMu   sync.RWMutex
	diskSizeCacheTTL  = 30 * time.Second
)

// getDirSizeBytes returns the actual disk usage of a directory in bytes using du command
// Results are cached for 30 seconds to avoid expensive repeated calls
func getDirSizeBytes(path string) (int64, error) {
	diskSizeCacheMu.RLock()
	if cachedSize, ok := diskSizeCache[path]; ok {
		if time.Since(diskSizeCacheTime[path]) < diskSizeCacheTTL {
			diskSizeCacheMu.RUnlock()
			return cachedSize, nil
		}
	}
	diskSizeCacheMu.RUnlock()

	cmd := exec.Command("du", "-sk", path)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(output))
	if len(fields) < 1 {
		return 0, fmt.Errorf("unexpected du output")
	}

	var sizeKB int64
	fmt.Sscanf(fields[0], "%d", &sizeKB)
	size := sizeKB * 1024

	diskSizeCacheMu.Lock()
	diskSizeCache[path] = size
	diskSizeCacheTime[path] = time.Now()
	diskSizeCacheMu.Unlock()

	return size, nil
}
