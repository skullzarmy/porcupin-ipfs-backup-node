package logging

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenLogFile opens (or creates/appends) today's log file at dataDir/logs/porcupin-YYYY-MM-DD.log.
// It also deletes log files older than retainDays. Returns a non-nil file on success.
// A log file failure is non-fatal — callers should continue without file logging on error.
func OpenLogFile(dataDir string, retainDays int) (*os.File, error) {
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, err
	}

	// Rotate: delete files older than retainDays
	cutoff := time.Now().AddDate(0, 0, -retainDays)
	if entries, err := os.ReadDir(logsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Only rotate our own log files (porcupin-*.log)
			if !strings.HasPrefix(name, "porcupin-") || !strings.HasSuffix(name, ".log") {
				continue
			}
			if info, err := entry.Info(); err == nil && info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(logsDir, name))
			}
		}
	}

	// Open today's log file (append mode)
	filename := "porcupin-" + time.Now().Format("2006-01-02") + ".log"
	path := filepath.Join(logsDir, filename)
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}
