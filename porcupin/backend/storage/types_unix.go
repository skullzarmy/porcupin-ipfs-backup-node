//go:build !windows
// +build !windows

package storage

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// getDirSize calculates the total size of a directory using du command (Unix)
func getDirSize(path string) (int64, error) {
	log.Printf("Calculating size of %s...", path)
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
	log.Printf("Size of %s: %d bytes (%.2f GB)", path, size, float64(size)/1024/1024/1024)
	return size, nil
}

// rsyncMigrate performs migration using rsync with real progress tracking (Unix)
func (m *Manager) rsyncMigrate(ctx context.Context, source, dest string, totalSize int64, progressCallback func(MigrationStatus)) error {
	// Create destination directory (not just parent)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// rsync flags:
	// -a = archive mode (preserves permissions, timestamps, etc.)
	// -v = verbose
	// --progress = show per-file progress
	// --partial = keep partially transferred files for resume
	// --timeout=300 = I/O timeout to detect stalled transfers (5 min)
	// --exclude = skip lock files (we don't want stale locks at destination)
	//
	// Note: macOS ships with old rsync (2.6.9) that doesn't support --info=progress2
	args := []string{
		"-av",
		"--progress",
		"--partial",
		"--timeout=300",
		"--exclude=repo.lock",
		"--exclude=*.lock",
		source + "/",
		dest + "/",
	}

	log.Printf("Running: rsync %s", strings.Join(args, " "))

	// DON'T use CommandContext - we don't want app context cancellation to kill rsync
	// The migration should complete even if the user navigates away
	// But we store a reference so it can be cancelled if user explicitly requests
	cmd := exec.Command("rsync", args...)

	// Store reference for cancellation
	m.mu.Lock()
	m.rsyncCmd = cmd
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.rsyncCmd = nil
		m.mu.Unlock()
	}()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start rsync: %w", err)
	}

	log.Printf("rsync started with PID %d", cmd.Process.Pid)

	// Track bytes copied by parsing rsync output
	var bytesCopied int64

	// Parse rsync per-file progress
	// Format: "   1234567 100%   12.34MB/s    0:00:01 (xfer#1, to-check=99/100)"
	progressRegex := regexp.MustCompile(`^\s*([\d,]+)\s+(\d+)%`)
	// Also match file names being transferred
	fileRegex := regexp.MustCompile(`^([^/\s].+)$`)

	// Use WaitGroups to ensure goroutines complete before we check results
	var wg sync.WaitGroup
	var stderrBuf strings.Builder
	var stdoutErr, stderrErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		// Increase buffer size for large file names
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		scanner.Split(bufio.ScanLines)
		var currentFile string
		lineCount := 0
		lastLogTime := time.Now()
		for scanner.Scan() {
			line := scanner.Text()
			lineCount++

			// Log periodically so we know rsync is still running (every 30 seconds or 1000 lines)
			if lineCount%1000 == 0 || time.Since(lastLogTime) > 30*time.Second {
				log.Printf("rsync progress: processed %d lines, copied %.2f GB", lineCount, float64(bytesCopied)/1024/1024/1024)
				lastLogTime = time.Now()
			}

			// Check for file name (lines that don't start with whitespace and aren't progress)
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\r") {
				if matches := fileRegex.FindStringSubmatch(strings.TrimSpace(line)); len(matches) > 0 {
					currentFile = matches[1]
				}
			}

			// Check for progress line
			if matches := progressRegex.FindStringSubmatch(line); len(matches) >= 3 {
				bytesStr := strings.ReplaceAll(matches[1], ",", "")
				fileBytes, _ := strconv.ParseInt(bytesStr, 10, 64)
				filePct, _ := strconv.Atoi(matches[2])

				// When a file completes (100%), add its size to total
				if filePct == 100 {
					bytesCopied += fileBytes
				}

				// Calculate overall progress based on bytes copied
				var overallPct float64
				if totalSize > 0 {
					overallPct = float64(bytesCopied) / float64(totalSize) * 100
					if overallPct > 100 {
						overallPct = 100
					}
				}

				m.mu.Lock()
				m.migrationStatus.BytesCopied = bytesCopied
				m.migrationStatus.Progress = overallPct
				m.migrationStatus.CurrentFile = currentFile
				status := *m.migrationStatus
				m.mu.Unlock()

				if progressCallback != nil {
					progressCallback(status)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			stdoutErr = err
			log.Printf("rsync stdout scanner error: %v", err)
		}
		log.Printf("rsync stdout reader finished after %d lines", lineCount)
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line + "\n")
			log.Printf("rsync stderr: %s", line)
		}
		if err := scanner.Err(); err != nil {
			stderrErr = err
			log.Printf("rsync stderr scanner error: %v", err)
		}
	}()

	// Wait for rsync to complete
	log.Printf("Waiting for rsync to complete...")
	waitErr := cmd.Wait()

	// Wait for goroutines to finish reading
	wg.Wait()

	// Log completion status
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		log.Printf("rsync exited with error (code %d): %v", exitCode, waitErr)
		log.Printf("rsync stderr output: %s", stderrBuf.String())

		m.mu.Lock()
		m.migrationStatus.Error = stderrBuf.String()
		m.mu.Unlock()
		return fmt.Errorf("rsync failed (exit code %d): %w\nstderr: %s", exitCode, waitErr, stderrBuf.String())
	}

	if stdoutErr != nil {
		log.Printf("Warning: stdout read error: %v", stdoutErr)
	}
	if stderrErr != nil {
		log.Printf("Warning: stderr read error: %v", stderrErr)
	}

	log.Printf("rsync completed successfully! Copied %.2f GB", float64(bytesCopied)/1024/1024/1024)

	m.mu.Lock()
	m.migrationStatus.Progress = 100
	m.migrationStatus.BytesCopied = totalSize
	m.mu.Unlock()

	return nil
}
