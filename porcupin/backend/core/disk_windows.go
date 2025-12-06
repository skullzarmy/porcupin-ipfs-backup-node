//go:build windows
// +build windows

package core

import (
	"log"
	"syscall"
	"unsafe"
)

// hasSufficientDiskSpace checks if there's enough free disk space on the IPFS repo volume (Windows implementation)
func (bm *BackupManager) hasSufficientDiskSpace() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable uint64
	var totalNumberOfBytes uint64
	var totalNumberOfFreeBytes uint64

	// Check the drive containing the IPFS repository
	// This correctly handles external drives, network shares, etc.
	repoPath := bm.ipfs.GetRepoPath()
	pathToCheck := "C:\\" // Default fallback
	
	if len(repoPath) >= 2 && repoPath[1] == ':' {
		// Extract drive letter from path (e.g., "D:\ipfs" -> "D:\")
		pathToCheck = string(repoPath[0]) + ":\\"
	} else if len(repoPath) >= 2 && repoPath[0] == '\\' && repoPath[1] == '\\' {
		// UNC path (e.g., \\server\share) - use full path for GetDiskFreeSpaceEx
		pathToCheck = repoPath
	}

	path, err := syscall.UTF16PtrFromString(pathToCheck)
	if err != nil {
		log.Printf("Failed to convert path %s: %v", pathToCheck, err)
		return true // Fail open
	}

	ret, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)

	if ret == 0 {
		log.Printf("Failed to check disk space for %s: %v", pathToCheck, callErr)
		return true // Fail open
	}

	// Calculate free space in GB
	freeGB := float64(freeBytesAvailable) / (1024 * 1024 * 1024)
	minFree := float64(bm.config.Backup.MinFreeDiskSpaceGB)

	if freeGB < minFree {
		log.Printf("Low disk space on %s: %.2f GB free (minimum: %.2f GB)", pathToCheck, freeGB, minFree)
		return false
	}

	return true
}
