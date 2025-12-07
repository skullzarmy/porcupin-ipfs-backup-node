//go:build windows
// +build windows

package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// getDirSize calculates the total size of a directory using robocopy (Windows)
func getDirSize(path string) (int64, error) {
	log.Printf("Calculating size of %s...", path)

	// Use robocopy in list-only mode to get directory size
	// /L = List only, /S = include subdirectories, /NFL /NDL /NJH /NJS = minimize output
	// /BYTES = show sizes in bytes
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

	log.Printf("Size of %s: %d bytes (%.2f GB)", path, size, float64(size)/1024/1024/1024)
	return size, nil
}

// getDirSizePowerShell is a fallback method using PowerShell
func getDirSizePowerShell(path string) (int64, error) {
	psCmd := `(Get-ChildItem -Path '` + strings.ReplaceAll(path, "'", "''") + `' -Recurse -Force -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)

	output, err := cmd.Output()
	if err != nil {
		log.Printf("PowerShell fallback also failed: %v", err)
		return 0, err
	}

	sizeStr := strings.TrimSpace(string(output))
	size, _ := strconv.ParseInt(sizeStr, 10, 64)

	log.Printf("Size of %s: %d bytes (%.2f GB)", path, size, float64(size)/1024/1024/1024)
	return size, nil
}

// rsyncMigrate performs migration using robocopy (Windows)
func (m *Manager) rsyncMigrate(ctx context.Context, source, dest string, totalSize int64, progressCallback func(MigrationStatus)) error {
	// Create destination directory
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Use robocopy for Windows migration
	// /E = copy subdirectories including empty ones
	// /COPYALL = copy all file information (timestamps, permissions, etc.)
	// /R:3 = retry 3 times on failed copies
	// /W:5 = wait 5 seconds between retries
	// /XF = exclude files (lock files)
	// /BYTES = show sizes in bytes (enables per-file byte counts in output)
	// /V = verbose output (shows file names being copied)
	// /ETA = show estimated time of arrival for copied files
	args := []string{
		source,
		dest,
		"/E",
		"/COPYALL",
		"/R:3",
		"/W:5",
		"/XF", "repo.lock", "*.lock",
		"/BYTES",
		"/V",
	}

	log.Printf("Running: robocopy %s", strings.Join(args, " "))

	cmd := exec.Command("robocopy", args...)

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
		return fmt.Errorf("failed to start robocopy: %w", err)
	}

	log.Printf("robocopy started with PID %d", cmd.Process.Pid)

	var wg sync.WaitGroup
	var stderrBuf strings.Builder
	var bytesCopied int64
	var currentFile string

	wg.Add(2)

	// Parse robocopy output for progress
	// With /BYTES /V, output includes lines like:
	//   "New File  \t\t      12345\tfilename.ext"
	//   "100%"
	go func() {
		defer wg.Done()
		buf := make([]byte, 8192)
		var lineBuf strings.Builder
		
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				
				// Process line by line for accurate byte counting
				for _, char := range chunk {
					if char == '\n' || char == '\r' {
						line := lineBuf.String()
						lineBuf.Reset()
						
						if line == "" {
							continue
						}
						
						// Parse "New File" or "Newer" lines which contain file size
						// Format: "  New File  \t\t   123456789\tpath\\to\\file.ext"
						if strings.Contains(line, "New File") || strings.Contains(line, "Newer") || 
						   strings.Contains(line, "Modified") || strings.Contains(line, "*EXTRA") {
							// Extract the file size (number before the filename)
							fields := strings.Fields(line)
							for i, field := range fields {
								// Look for a numeric field that represents file size
								if size, parseErr := strconv.ParseInt(field, 10, 64); parseErr == nil && size > 0 {
									// The next field after size is usually the filename
									if i+1 < len(fields) {
										currentFile = fields[i+1]
									}
									bytesCopied += size
									
									// Calculate progress
									var progress float64
									if totalSize > 0 {
										progress = float64(bytesCopied) / float64(totalSize) * 100
										if progress > 100 {
											progress = 100
										}
									}
									
									m.mu.Lock()
									m.migrationStatus.BytesCopied = bytesCopied
									m.migrationStatus.Progress = progress
									m.migrationStatus.CurrentFile = currentFile
									status := *m.migrationStatus
									m.mu.Unlock()
									
									if progressCallback != nil {
										progressCallback(status)
									}
									break
								}
							}
						}
						
						// Log progress periodically
						if strings.Contains(line, "Bytes :") || strings.Contains(line, "Files :") {
							log.Printf("robocopy: %s", line)
						}
					} else {
						lineBuf.WriteRune(char)
					}
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("robocopy stdout read error: %v", err)
				break
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				stderrBuf.WriteString(line)
				log.Printf("robocopy stderr: %s", strings.TrimSpace(line))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("robocopy stderr read error: %v", err)
				break
			}
		}
	}()

	// Wait for robocopy to complete
	log.Printf("Waiting for robocopy to complete...")
	waitErr := cmd.Wait()

	// Wait for goroutines to finish reading
	wg.Wait()

	// robocopy exit codes: 0-7 are success, 8+ are errors
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	if exitCode >= 8 {
		log.Printf("robocopy exited with error (code %d): %v", exitCode, waitErr)
		log.Printf("robocopy stderr output: %s", stderrBuf.String())

		m.mu.Lock()
		m.migrationStatus.Error = stderrBuf.String()
		m.mu.Unlock()
		return fmt.Errorf("robocopy failed (exit code %d): %w\nstderr: %s", exitCode, waitErr, stderrBuf.String())
	}

	log.Printf("robocopy completed successfully (exit code %d), copied %.2f GB", exitCode, float64(bytesCopied)/1024/1024/1024)

	m.mu.Lock()
	m.migrationStatus.Progress = 100
	m.migrationStatus.BytesCopied = totalSize
	m.mu.Unlock()

	return nil
}
