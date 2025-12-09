//go:build windows
// +build windows

package core

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// GetDiskUsageBytes calculates directory size using robocopy (Windows)
func GetDiskUsageBytes(path string) (int64, error) {
	// Use robocopy in list-only mode
	cmd := exec.Command("robocopy", path, "NUL", "/L", "/S", "/NFL", "/NDL", "/NJH", "/BYTES")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// robocopy exit codes 0-7 are success
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() < 8 {
				err = nil
			}
		}
	}

	if err != nil {
		// Fallback to PowerShell
		return getDiskUsagePowerShell(path)
	}

	// Parse "Bytes : <number>"
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

	log.Printf("Disk usage of %s: %.2f GB", path, float64(size)/1024/1024/1024)
	return size, nil
}

// getDiskUsagePowerShell is a fallback using PowerShell
func getDiskUsagePowerShell(path string) (int64, error) {
	psCmd := `(Get-ChildItem -Path '` + strings.ReplaceAll(path, "'", "''") + `' -Recurse -Force -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("PowerShell fallback failed: %w", err)
	}

	sizeStr := strings.TrimSpace(string(output))
	size, _ := strconv.ParseInt(sizeStr, 10, 64)

	log.Printf("Disk usage of %s: %.2f GB", path, float64(size)/1024/1024/1024)
	return size, nil
}
