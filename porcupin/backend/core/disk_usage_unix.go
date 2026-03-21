//go:build !windows
// +build !windows

package core

import (
	"fmt"
	"log/slog"
	"os/exec"
)

// GetDiskUsageBytes calculates directory size using du command (Unix)
func GetDiskUsageBytes(path string) (int64, error) {
	cmd := exec.Command("du", "-sk", path)
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("Disk usage check failed", "path", path, "error", err)
		return 0, nil
	}

	var sizeKB int64
	if _, err := fmt.Sscanf(string(output), "%d", &sizeKB); err != nil {
		return 0, fmt.Errorf("failed to parse du output: %w", err)
	}

	slog.Debug("Disk usage", "path", path, "gb", float64(sizeKB)/1024/1024)
	return sizeKB * 1024, nil
}
