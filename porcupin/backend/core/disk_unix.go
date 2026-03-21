//go:build !windows
// +build !windows

package core

import (
	"log/slog"
	"syscall"
)

// hasSufficientDiskSpace checks if there's enough free disk space on the IPFS repo volume (Unix implementation)
func (bm *BackupManager) hasSufficientDiskSpace() bool {
	// Check free space on the volume containing the IPFS repository
	// This correctly handles external drives, NAS, and other mount points
	repoPath := bm.ipfs.GetRepoPath()
	if repoPath == "" {
		repoPath = "/" // Fallback to root if no repo path
	}

	var stat syscall.Statfs_t
	err := syscall.Statfs(repoPath, &stat)
	if err != nil {
		slog.Error("Failed to check disk space", "path", repoPath, "error", err)
		return true // Fail open
	}

	// Calculate free space in GB
	// Use Bavail (blocks available to non-root) not Bfree (includes reserved blocks)
	freeGB := float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	minFree := float64(bm.config.Backup.MinFreeDiskSpaceGB)

	if freeGB < minFree {
		slog.Warn("Low disk space", "path", repoPath, "free_gb", freeGB, "min_free_gb", minFree)
		return false
	}

	return true
}
