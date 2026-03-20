package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"porcupin/backend/config"
	"porcupin/backend/ipfs"
	"porcupin/backend/logging"
	"porcupin/backend/storage"
	"porcupin/backend/version"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// StorageInfo represents storage usage information
type StorageInfo struct {
	UsedBytes       int64   `json:"used_bytes"`        // From database (sum of asset sizes)
	UsedGB          float64 `json:"used_gb"`           // From database
	DiskUsageBytes  int64   `json:"disk_usage_bytes"`  // Actual IPFS repo size on disk
	DiskUsageGB     float64 `json:"disk_usage_gb"`     // Actual IPFS repo size on disk
	MaxStorageGB    int     `json:"max_storage_gb"`
	WarningPct      int     `json:"warning_pct"`
	UsagePct        float64 `json:"usage_pct"`
	IsWarning       bool    `json:"is_warning"`
	IsLimitReached  bool    `json:"is_limit_reached"`
	FreeDiskSpaceGB float64 `json:"free_disk_space_gb"`
	RepoPath        string  `json:"repo_path"`
}

// ClearDataStatus represents the progress of clearing all data
type ClearDataStatus struct {
	InProgress    bool   `json:"in_progress"`
	Phase         string `json:"phase"` // "unpinning", "garbage_collect", "clearing_db", "complete", "error"
	Message       string `json:"message"`
	TotalPins     int    `json:"total_pins"`
	UnpinnedCount int    `json:"unpinned_count"`
	Error         string `json:"error,omitempty"`
}

// GetConfig returns the current configuration
func (a *App) GetConfig() config.Config {
	return *a.config
}

// GetStorageInfo returns current storage usage information
func (a *App) GetStorageInfo() (StorageInfo, error) {
	info := StorageInfo{
		MaxStorageGB: a.config.Backup.MaxStorageGB,
		WarningPct:   a.config.Backup.StorageWarningPct,
		RepoPath:     a.ipfsNode.GetRepoPath(),
	}

	// Get total size of pinned assets from database
	stats, err := a.database.GetAssetStats()
	if err != nil {
		return info, err
	}
	info.UsedBytes = stats["total_size_bytes"]
	info.UsedGB = float64(info.UsedBytes) / (1024 * 1024 * 1024)

	// Get cached disk usage from DB (updated on pin/unpin/migration)
	diskUsageStr, _ := a.database.GetSetting("disk_usage_bytes")
	if diskUsageStr != "" {
		var diskUsage int64
		fmt.Sscanf(diskUsageStr, "%d", &diskUsage)
		info.DiskUsageBytes = diskUsage
		info.DiskUsageGB = float64(diskUsage) / (1024 * 1024 * 1024)
	}

	// Calculate usage percentage if max is set (use disk usage for accuracy)
	if info.MaxStorageGB > 0 {
		info.UsagePct = (info.DiskUsageGB / float64(info.MaxStorageGB)) * 100
		info.IsWarning = info.UsagePct >= float64(info.WarningPct)
		info.IsLimitReached = info.UsagePct >= 100
	}

	// Get free disk space
	info.FreeDiskSpaceGB = getFreeDiskSpaceGB()

	return info, nil
}

// UpdateSettings updates the application settings
func (a *App) UpdateSettings(settings map[string]interface{}) error {
	// Update config values
	if v, ok := settings["max_storage_gb"].(float64); ok {
		a.config.Backup.MaxStorageGB = int(v)
	}
	if v, ok := settings["storage_warning_pct"].(float64); ok {
		a.config.Backup.StorageWarningPct = int(v)
	}
	if v, ok := settings["max_concurrency"].(float64); ok {
		a.config.Backup.MaxConcurrency = int(v)
		// Note: takes effect on next app restart (worker pool is sized at init)
	}
	if v, ok := settings["min_free_disk_space_gb"].(float64); ok {
		a.config.Backup.MinFreeDiskSpaceGB = int(v)
	}
	if v, ok := settings["max_file_size_gb"].(float64); ok {
		a.config.IPFS.MaxFileSize = int64(v * 1024 * 1024 * 1024)
	}
	if v, ok := settings["pin_timeout_minutes"].(float64); ok {
		a.config.IPFS.PinTimeout = time.Duration(v) * time.Minute
	}
	if v, ok := settings["sync_owned"].(bool); ok {
		a.config.Backup.SyncOwned = v
	}
	if v, ok := settings["sync_created"].(bool); ok {
		a.config.Backup.SyncCreated = v
	}
	// Note: ipfs_swarm_port is saved but requires app restart to take effect
	if v, ok := settings["ipfs_swarm_port"].(float64); ok {
		port := int(v)
		if port >= 1024 && port <= 65535 {
			a.config.IPFS.SwarmPort = port
		}
	}

	// Save config to file
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".porcupin", "config.yaml")
	return a.config.SaveConfig(configPath)
}

// RecoverMissingAssets triggers the verification and repair process for missing asset records
func (a *App) RecoverMissingAssets() (map[string]int, error) {
	return a.backupService.VerifyAndFixPins()
}

// ResetDatabase clears all NFTs, assets, and unpins all IPFS content
func (a *App) ResetDatabase() error {
	slog.Info("Starting full data reset")

	// Emit starting event
	wailsRuntime.EventsEmit(a.ctx, "clear:start", ClearDataStatus{
		InProgress: true,
		Phase:      "unpinning",
		Message:    "Starting reset...",
	})

	err := a.backupService.ClearAllData(func(phase, message string, total, current int) {
		wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
			InProgress:    true,
			Phase:         phase,
			Message:       message,
			TotalPins:     total,
			UnpinnedCount: current,
		})
	})

	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
			InProgress: true,
			Phase:      "error",
			Error:      err.Error(),
			Message:    "Reset failed",
		})
		return err
	}

	wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
		InProgress: false,
		Phase:      "complete",
		Message:    "Reset complete",
	})

	return nil
}

// GetIPFSRepoPath returns the path to the IPFS repository
func (a *App) GetIPFSRepoPath() string {
	return a.ipfsNode.GetRepoPath()
}

// GetStorageLocation returns information about the current storage location
func (a *App) GetStorageLocation() (*storage.StorageLocation, error) {
	repoPath := a.ipfsNode.GetRepoPath()
	return storage.GetStorageInfo(repoPath)
}

// ListStorageLocations returns all available storage locations
func (a *App) ListStorageLocations() ([]*storage.StorageLocation, error) {
	return storage.ListAvailableLocations()
}

// ValidateStoragePath checks if a path is valid for storage
func (a *App) ValidateStoragePath(path string) error {
	return storage.ValidatePath(path)
}

// GetStorageType detects what type of storage a path is
func (a *App) GetStorageType(path string) (string, error) {
	storageType, err := storage.DetectStorageType(path)
	if err != nil {
		return "", err
	}
	return string(storageType), nil
}

// MigrateStorage moves the IPFS repository to a new location
// This will stop the backup service, move the data, and restart with new location
func (a *App) MigrateStorage(destPath string) error {
	slog.Info("MigrateStorage called", "destination", destPath)

	// Validate destination first
	slog.Debug("Validating destination path")
	if err := storage.ValidatePath(destPath); err != nil {
		slog.Error("Destination validation failed", "error", err)
		return fmt.Errorf("invalid destination: %w", err)
	}
	slog.Debug("Destination validated successfully")

	// Get current path
	currentPath := a.ipfsNode.GetRepoPath()
	slog.Debug("Current IPFS path", "path", currentPath)

	// Check if same path
	expandedDest, _ := storage.ExpandPath(destPath)
	if currentPath == expandedDest {
		return fmt.Errorf("destination is same as current location")
	}

	// Emit starting event
	wailsRuntime.EventsEmit(a.ctx, "storage:migration:start", map[string]interface{}{
		"source": currentPath,
		"dest":   destPath,
	})

	// Stop backup service
	slog.Info("Stopping backup service for migration")
	a.backupService.Stop()

	// Stop IPFS node
	slog.Info("Stopping IPFS node for migration")
	if err := a.ipfsNode.Stop(); err != nil {
		a.backupService.Start(a.ctx) // Try to restart
		return fmt.Errorf("failed to stop IPFS node: %w", err)
	}
	slog.Info("IPFS node stopped, starting migration")

	// Create storage manager and perform migration
	manager := storage.NewManager(currentPath)

	err := manager.Migrate(a.ctx, destPath, func(status storage.MigrationStatus) {
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:progress", status)
	})

	if err != nil {
		// Try to restart with old path
		slog.Error("Migration failed, attempting to restart with old path", "error", err)
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:error", err.Error())

		newNode, nodeErr := ipfs.NewNode(currentPath, a.config.IPFS.SwarmPort)
		if nodeErr == nil {
			nodeErr = newNode.Start(a.ctx)
			if nodeErr == nil {
				a.ipfsNode = newNode
				a.backupService.UpdateIPFS(newNode)
			}
		}
		a.backupService.Start(a.ctx)
		return fmt.Errorf("migration failed: %w", err)
	}

	// Update config with new path
	a.config.IPFS.RepoPath = expandedDest
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".porcupin", "config.yaml")
	if err := a.config.SaveConfig(configPath); err != nil {
		slog.Warn("Failed to save config after migration", "error", err)
	}

	// Start IPFS node with new path
	slog.Info("Starting IPFS node at new location", "path", expandedDest)
	newNode, err := ipfs.NewNode(expandedDest, a.config.IPFS.SwarmPort)
	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:error", err.Error())
		return fmt.Errorf("failed to create node at new location: %w", err)
	}

	if err := newNode.Start(a.ctx); err != nil {
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:error", err.Error())
		return fmt.Errorf("failed to start node at new location: %w", err)
	}

	a.ipfsNode = newNode

	// Update IPFS refs in backup service/manager before restarting
	a.backupService.UpdateIPFS(newNode)

	// Restart backup service
	a.backupService.Start(a.ctx)

	// Update disk usage for new location
	a.backupService.GetManager().MarkDiskUsageDirty()
	a.backupService.GetManager().UpdateDiskUsage()

	wailsRuntime.EventsEmit(a.ctx, "storage:migration:complete", map[string]interface{}{
		"new_path": expandedDest,
	})

	slog.Info("Storage migration complete", "from", currentPath, "to", expandedDest)
	return nil
}

// GetMigrationStatus returns the current migration status
func (a *App) GetMigrationStatus() storage.MigrationStatus {
	return a.backupService.GetStorageManager().GetMigrationStatus()
}

// CancelMigration cancels an ongoing storage migration
func (a *App) CancelMigration() error {
	slog.Info("CancelMigration called")
	err := a.backupService.GetStorageManager().CancelMigration()
	if err != nil {
		slog.Error("CancelMigration failed", "error", err)
		return err
	}

	wailsRuntime.EventsEmit(a.ctx, "storage:migration:cancelled", nil)

	// Restart IPFS and backup service with original path
	slog.Info("Restarting services after cancellation")
	currentPath := a.ipfsNode.GetRepoPath()

	newNode, err := ipfs.NewNode(currentPath, a.config.IPFS.SwarmPort)
	if err != nil {
		slog.Error("Failed to create node after cancel", "error", err)
		return fmt.Errorf("failed to restart IPFS: %w", err)
	}

	if err := newNode.Start(a.ctx); err != nil {
		slog.Error("Failed to start node after cancel", "error", err)
		return fmt.Errorf("failed to restart IPFS: %w", err)
	}

	a.ipfsNode = newNode
	a.backupService.UpdateIPFS(newNode)
	a.backupService.Start(a.ctx)

	slog.Info("Services restarted after migration cancellation")
	return nil
}

// BrowseForFolder opens a folder picker dialog
func (a *App) BrowseForFolder() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Storage Location",
	})
}

// GetLogs returns recent log entries from the in-memory ring buffer.
// minLevel: "INFO", "WARN", "ERROR", or "" for all entries.
func (a *App) GetLogs(limit int, minLevel string) []logging.Entry {
	if a.logRing == nil {
		return nil
	}
	return a.logRing.Entries(limit, minLevel)
}

// ExportLogs returns all buffered log entries as formatted plain text.
func (a *App) ExportLogs() string {
	if a.logRing == nil {
		return ""
	}
	return a.logRing.ExportText()
}

// ExportDiagnosticReport bundles version, OS, logs, crash files, and stats into a plain text report.
func (a *App) ExportDiagnosticReport() string {
	var sb strings.Builder

	sb.WriteString("Porcupin Diagnostic Report\n")
	sb.WriteString("==========================\n\n")
	fmt.Fprintf(&sb, "App Version: %s\n", version.Version)
	fmt.Fprintf(&sb, "Go Version:  %s\n", runtime.Version())
	fmt.Fprintf(&sb, "OS/Arch:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&sb, "Generated:   %s\n\n", time.Now().Format(time.RFC3339))

	// Config summary (sensitive fields redacted)
	sb.WriteString("Configuration\n")
	sb.WriteString("-------------\n")
	if a.config != nil {
		fmt.Fprintf(&sb, "MaxStorageGB:       %d\n", a.config.Backup.MaxStorageGB)
		fmt.Fprintf(&sb, "MaxConcurrency:     %d\n", a.config.Backup.MaxConcurrency)
		fmt.Fprintf(&sb, "MinFreeDiskSpaceGB: %d\n", a.config.Backup.MinFreeDiskSpaceGB)
		fmt.Fprintf(&sb, "PinTimeout:         %s\n", a.config.IPFS.PinTimeout)
		fmt.Fprintf(&sb, "SwarmPort:          %d\n", a.config.IPFS.SwarmPort)
		fmt.Fprintf(&sb, "RepoPath:           %s\n", a.config.IPFS.RepoPath)
		authPassDisplay := ""
		if a.config.Server.AuthPass != "" {
			authPassDisplay = "[redacted]"
		}
		fmt.Fprintf(&sb, "AuthEnabled:        %v\n", a.config.Server.EnableAuth)
		fmt.Fprintf(&sb, "AuthPass:           %s\n", authPassDisplay)
	}
	sb.WriteString("\n")

	// Asset stats
	sb.WriteString("Asset Statistics\n")
	sb.WriteString("----------------\n")
	if a.database != nil {
		if stats, err := a.database.GetAssetStats(); err == nil {
			for k, v := range stats {
				fmt.Fprintf(&sb, "%s: %d\n", k, v)
			}
		} else {
			fmt.Fprintf(&sb, "Error reading stats: %v\n", err)
		}
	}
	sb.WriteString("\n")

	// In-memory log buffer
	sb.WriteString("Recent Logs (ring buffer)\n")
	sb.WriteString("-------------------------\n")
	if a.logRing != nil {
		sb.WriteString(a.logRing.ExportText())
	} else {
		sb.WriteString("(logging not initialized)\n")
	}
	sb.WriteString("\n")

	// Crash reports
	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		fmt.Fprintf(&sb, "Could not locate crash reports: %v\n", homeErr)
	} else {
		logsDir := filepath.Join(homeDir, ".porcupin", "logs")
		crashFiles, _ := filepath.Glob(filepath.Join(logsDir, "crash-*.txt"))
		if len(crashFiles) > 0 {
			for _, cf := range crashFiles {
				fmt.Fprintf(&sb, "Crash Report: %s\n", filepath.Base(cf))
				sb.WriteString("---\n")
				data, err := os.ReadFile(cf)
				if err != nil {
					fmt.Fprintf(&sb, "(error reading: %v)\n", err)
				} else {
					sb.Write(data)
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("No crash reports found.\n")
		}
	}

	return sb.String()
}

// ExportLogsToFile opens a save dialog and writes the log buffer to the chosen path.
// Returns the saved path, or "" if the user cancelled, or an error.
func (a *App) ExportLogsToFile() (string, error) {
	if a.logRing == nil {
		return "", fmt.Errorf("logging not initialized")
	}
	text := a.logRing.ExportText()
	defaultName := fmt.Sprintf("porcupin-logs-%s.txt", time.Now().Format("2006-01-02"))
	path, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Save Log File",
		DefaultFilename: defaultName,
	})
	if err != nil {
		return "", fmt.Errorf("dialog error: %w", err)
	}
	if path == "" {
		return "", nil // user cancelled
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", fmt.Errorf("failed to write log file: %w", err)
	}
	return path, nil
}

// ExportDiagnosticReportToFile opens a save dialog and writes the diagnostic report to the chosen path.
// Returns the saved path, or "" if the user cancelled, or an error.
func (a *App) ExportDiagnosticReportToFile() (string, error) {
	text := a.ExportDiagnosticReport()
	defaultName := fmt.Sprintf("porcupin-diagnostic-%s.txt", time.Now().Format("2006-01-02"))
	path, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Save Diagnostic Report",
		DefaultFilename: defaultName,
	})
	if err != nil {
		return "", fmt.Errorf("dialog error: %w", err)
	}
	if path == "" {
		return "", nil // user cancelled
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", fmt.Errorf("failed to write diagnostic report: %w", err)
	}
	return path, nil
}
