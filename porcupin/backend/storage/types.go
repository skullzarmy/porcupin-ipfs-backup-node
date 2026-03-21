package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StorageType indicates the type of storage location
type StorageType string

const (
	StorageTypeLocal    StorageType = "local"    // Local filesystem (same disk)
	StorageTypeExternal StorageType = "external" // External drive (USB, SD card)
	StorageTypeNetwork  StorageType = "network"  // Network storage (SMB, NFS)
)

// StorageLocation represents a storage location with metadata
type StorageLocation struct {
	Path       string      `json:"path" yaml:"path"`
	Type       StorageType `json:"type" yaml:"type"`
	Label      string      `json:"label" yaml:"label"`
	TotalBytes int64       `json:"total_bytes" yaml:"total_bytes"`
	FreeBytes  int64       `json:"free_bytes" yaml:"free_bytes"`
	IsWritable bool        `json:"is_writable" yaml:"is_writable"`
	IsMounted  bool        `json:"is_mounted" yaml:"is_mounted"`
	MountPoint string      `json:"mount_point" yaml:"mount_point"`
	NetworkURI string      `json:"network_uri" yaml:"network_uri"`
}

// MigrationStatus tracks the progress of a storage migration
type MigrationStatus struct {
	InProgress  bool    `json:"in_progress"`
	SourcePath  string  `json:"source_path"`
	DestPath    string  `json:"dest_path"`
	Progress    float64 `json:"progress"`
	BytesCopied int64   `json:"bytes_copied"`
	TotalBytes  int64   `json:"total_bytes"`
	CurrentFile string  `json:"current_file"`
	Error       string  `json:"error,omitempty"`
	Method      string  `json:"method"`
	Phase       string  `json:"phase"` // "preparing", "copying", "verifying", "cleanup", "complete", "cancelled"
}

// Manager handles storage location management and migration
type Manager struct {
	mu              sync.RWMutex
	currentPath     string
	migrationStatus *MigrationStatus
	cancelFunc      context.CancelFunc // To cancel ongoing migration
	rsyncCmd        *exec.Cmd          // Reference to rsync process for cancellation
}


// NewManager creates a new storage manager
func NewManager(currentPath string) *Manager {
	return &Manager{
		currentPath:     currentPath,
		migrationStatus: &MigrationStatus{},
	}
}


// CancelMigration cancels an ongoing migration
func (m *Manager) CancelMigration() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.migrationStatus == nil || !m.migrationStatus.InProgress {
		return fmt.Errorf("no migration in progress")
	}
	
	slog.Info("Cancelling migration...")

	// Kill rsync process if running
	if m.rsyncCmd != nil && m.rsyncCmd.Process != nil {
		slog.Info("Killing rsync process", "pid", m.rsyncCmd.Process.Pid)
		if err := m.rsyncCmd.Process.Kill(); err != nil {
			slog.Warn("Failed to kill rsync", "error", err)
		}
	}
	
	// Cancel context if set
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	
	m.migrationStatus.Phase = "cancelled"
	m.migrationStatus.Error = "Migration cancelled by user"
	m.migrationStatus.InProgress = false
	
	return nil
}

// GetCurrentPath returns the current storage path
func (m *Manager) GetCurrentPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentPath
}

// GetMigrationStatus returns the current migration status
func (m *Manager) GetMigrationStatus() MigrationStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.migrationStatus == nil {
		return MigrationStatus{}
	}
	return *m.migrationStatus
}

// isWritable checks if a path is writable with timeout
func isWritable(path string) bool {
	testFile := filepath.Join(path, ".porcupin_write_test")
	
	// Use a channel to implement timeout
	done := make(chan bool, 1)
	go func() {
		f, err := os.Create(testFile)
		if err != nil {
			done <- false
			return
		}
		f.Close()
		os.Remove(testFile)
		done <- true
	}()
	
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		slog.Warn("Write test timed out", "path", path)
		return false
	}
}

// getDirSize calculates the total size of a directory
// Platform-specific implementations in types_unix.go and types_windows.go

// ExpandPath expands ~ to home directory
func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil
}

// Migrate moves the IPFS repository to a new location
func (m *Manager) Migrate(ctx context.Context, destPath string, progressCallback func(MigrationStatus)) error {
	slog.Info("Migrate called", "dest_path", destPath)
	
	m.mu.Lock()
	if m.migrationStatus != nil && m.migrationStatus.InProgress {
		m.mu.Unlock()
		slog.Info("Migration: already in progress, rejecting")
		return fmt.Errorf("migration already in progress")
	}

	sourcePath := m.currentPath
	slog.Info("Migration: paths determined", "source", sourcePath, "dest", destPath)
	
	m.migrationStatus = &MigrationStatus{
		InProgress: true,
		SourcePath: sourcePath,
		DestPath:   destPath,
		Phase:      "preparing",
	}
	
	m.mu.Unlock()

	// Helper to update status and callback
	updateStatus := func(updates func(*MigrationStatus)) {
		m.mu.Lock()
		updates(m.migrationStatus)
		status := *m.migrationStatus
		m.mu.Unlock()
		if progressCallback != nil {
			progressCallback(status)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error("PANIC in migration", "error", r)
			m.mu.Lock()
			m.migrationStatus.InProgress = false
			m.migrationStatus.Error = fmt.Sprintf("panic: %v", r)
			m.mu.Unlock()
		} else {
			m.mu.Lock()
			m.migrationStatus.InProgress = false
			m.mu.Unlock()
		}
		slog.Debug("Migration: defer executed, InProgress=false")
	}()

	// Expand destination path
	var err error
	destPath, err = ExpandPath(destPath)
	if err != nil {
		return err
	}

	// Always create a porcupin-ipfs subfolder at the destination
	// This prevents mixing IPFS data with user files
	if !strings.HasSuffix(destPath, "ipfs") && !strings.HasSuffix(destPath, "porcupin-ipfs") {
		destPath = filepath.Join(destPath, "porcupin-ipfs")
		slog.Info("Migration: will create subfolder", "path", destPath)
	}

	slog.Info("Migration: checking destination", "path", destPath)
	updateStatus(func(s *MigrationStatus) {
		s.Phase = "preparing"
		s.CurrentFile = "Checking destination..."
		s.DestPath = destPath // Update with actual destination path
	})

	// Check if destination is valid
	destInfo, err := GetStorageInfo(destPath)
	if err != nil {
		slog.Error("Migration: GetStorageInfo failed", "error", err)
		return fmt.Errorf("cannot access destination: %w", err)
	}

	slog.Info("Migration: destination info",
		"writable", destInfo.IsWritable,
		"mounted", destInfo.IsMounted,
		"free_gb", float64(destInfo.FreeBytes)/1024/1024/1024)

	if !destInfo.IsWritable {
		slog.Error("Migration: destination is not writable")
		return fmt.Errorf("destination is not writable")
	}

	slog.Info("Migration: calculating source size...")
	updateStatus(func(s *MigrationStatus) {
		s.CurrentFile = "Calculating source size..."
	})

	// Calculate source size
	sourceSize, err := getDirSize(sourcePath)
	if err != nil {
		return fmt.Errorf("cannot calculate source size: %w", err)
	}

	if destInfo.FreeBytes < sourceSize {
		return fmt.Errorf("insufficient space: need %.2f GB, have %.2f GB",
			float64(sourceSize)/1024/1024/1024,
			float64(destInfo.FreeBytes)/1024/1024/1024)
	}

	updateStatus(func(s *MigrationStatus) {
		s.TotalBytes = sourceSize
	})

	// Determine migration method
	sameDevice, err := SameDevice(sourcePath, destPath)
	if err != nil {
		sameDevice = false
	}

	if sameDevice {
		slog.Info("Migration: same device, using rename")
		updateStatus(func(s *MigrationStatus) {
			s.Method = "rename"
			s.Phase = "copying"
			s.CurrentFile = "Moving files (instant)..."
		})

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}

		if err := os.Rename(sourcePath, destPath); err != nil {
			return fmt.Errorf("failed to move: %w", err)
		}

		updateStatus(func(s *MigrationStatus) {
			s.Progress = 100
			s.BytesCopied = sourceSize
			s.Phase = "complete"
		})
		
		m.mu.Lock()
		m.currentPath = destPath
		m.mu.Unlock()
	} else {
		slog.Info("Migration: cross-device, using rsync")
		updateStatus(func(s *MigrationStatus) {
			s.Method = "rsync"
			s.Phase = "copying"
		})

		if err := m.rsyncMigrate(ctx, sourcePath, destPath, sourceSize, progressCallback); err != nil {
			return err
		}

		slog.Info("Migration: rsync complete, cleaning up source")
		updateStatus(func(s *MigrationStatus) {
			s.Phase = "cleanup"
			s.CurrentFile = "Removing source files..."
		})

		if err := os.RemoveAll(sourcePath); err != nil {
			slog.Warn("Failed to remove source after migration", "error", err)
		}

		m.mu.Lock()
		m.currentPath = destPath
		m.mu.Unlock()
	}

	updateStatus(func(s *MigrationStatus) {
		s.Phase = "complete"
		s.Progress = 100
	})

	return nil
}

// rsyncMigrate performs migration using platform-specific copy tool
// Unix: rsync, Windows: robocopy
// Platform-specific implementations in types_unix.go and types_windows.go

// ValidatePath checks if a path is valid for storage
func ValidatePath(path string) error {
	path, err := ExpandPath(path)
	if err != nil {
		return fmt.Errorf("cannot expand path: %w", err)
	}

	parentDir := filepath.Dir(path)
	parentCreated := false
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		// Try to create parent to verify we have permission
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("cannot create parent directory: %w", err)
		}
		parentCreated = true
	}

	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			if parentCreated {
				os.Remove(parentDir)
			}
			return fmt.Errorf("path exists but is not a directory")
		}
	}

	// Determine which directory to test for writability
	testDir := parentDir
	if _, err := os.Stat(path); err == nil {
		testDir = path
	}

	writable := isWritable(testDir)
	
	// Clean up if we created the parent just for testing
	if parentCreated {
		os.RemoveAll(parentDir)
	}

	if !writable {
		return fmt.Errorf("path is not writable (or timed out)")
	}

	return nil
}

// SameDevice checks if two paths are on the same device
func SameDevice(path1, path2 string) (bool, error) {
	var err error
	path1, err = ExpandPath(path1)
	if err != nil {
		return false, err
	}
	path2, err = ExpandPath(path2)
	if err != nil {
		return false, err
	}

	checkPath1 := path1
	if _, err := os.Stat(path1); os.IsNotExist(err) {
		checkPath1 = filepath.Dir(path1)
	}

	checkPath2 := path2
	if _, err := os.Stat(path2); os.IsNotExist(err) {
		checkPath2 = filepath.Dir(path2)
	}

	dev1, err := getDeviceID(checkPath1)
	if err != nil {
		return false, err
	}

	dev2, err := getDeviceID(checkPath2)
	if err != nil {
		return false, err
	}

	return dev1 == dev2, nil
}
