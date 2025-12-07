//go:build !windows
// +build !windows

package core

import (
	"fmt"
	"log"
	"os/exec"
)

// getDiskUsageBytes calculates directory size using du command (Unix)
func getDiskUsageBytes(path string) (int64, error) {
	cmd := exec.Command("du", "-sk", path)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("du command failed: %w", err)
	}

	var sizeKB int64
	if _, err := fmt.Sscanf(string(output), "%d", &sizeKB); err != nil {
		return 0, fmt.Errorf("failed to parse du output: %w", err)
	}

	log.Printf("Disk usage of %s: %.2f GB", path, float64(sizeKB)/1024/1024)
	return sizeKB * 1024, nil
}
