package updater

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/creativeprojects/go-selfupdate"
)

const (
	// Repository is the GitHub repository to check for updates
	Repository = "skullzarmy/porcupin-ipfs-backup-node"
)

// Manager handles the update process
type Manager struct {
	updater      *selfupdate.Updater
	currentVer   string
	latestRelease *selfupdate.Release
}

// NewManager creates a new update manager
func NewManager(currentVersion string) *Manager {
	// Configure updater
	// We want to verify assets if possible, but for now we'll start simple
	// selfupdate automatically detects OS/Arch
	
	up, err := selfupdate.NewUpdater(selfupdate.Config{
		Validator: &selfupdate.ChecksumValidator{
			UniqueFilename: "checksums.txt", // Standard GoReleaser checksum file
		},
	})
	
	if err != nil {
		log.Printf("Failed to create updater: %v", err)
		// Fallback to simpler updater if validator fails (e.g. config error)
		// but usually NewUpdater only errors on fundamental config
	}

	return &Manager{
		updater:    up,
		currentVer: currentVersion,
	}
}

// CheckForUpdates checks GitHub for a newer version
func (m *Manager) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	if m.updater == nil {
		return nil, fmt.Errorf("updater not initialized")
	}

	log.Printf("Checking for updates (current: %s)...", m.currentVer)
	
	// Detect latest version
	latest, found, err := m.updater.DetectLatest(ctx, selfupdate.ParseSlug(Repository))
	if err != nil {
		return nil, fmt.Errorf("failed to detect update: %w", err)
	}

	info := &UpdateInfo{
		CurrentVer: m.currentVer,
		Available:  false,
	}

	if !found {
		log.Println("No updates found.")
		return info, nil
	}

	// Compare versions
	if latest.LessOrEqual(m.currentVer) {
		log.Printf("Latest version %s is not newer than current %s", latest.Version(), m.currentVer)
		return info, nil
	}

	log.Printf("New version found: %s", latest.Version())
	
	// Cache the release for installation step
	m.latestRelease = latest

	// Populate info
	info.Available = true
	info.Version = latest.Version()
	info.ReleaseNotes = latest.ReleaseNotes
	info.PubDate = latest.PublishedAt
	info.AssetURL = latest.AssetURL
	
	// Helper to calculate size if available
	// The library might not expose raw asset size easily without iterating assets,
	// checking latest.AssetID or similar.
	// We'll leave HumanSize empty for now or populate if we can find the specific asset.
	
	return info, nil
}

// InstallLatest downloads and applies the cached latest update
func (m *Manager) InstallLatest(ctx context.Context) error {
	if m.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	if m.latestRelease == nil {
		return fmt.Errorf("no update available to install")
	}

	// Get the executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	
	// Resolve symbolic links if any (common on macOS homebrew etc)
	exePath, err := filepath.EvalSymlinks(exe)
	if err != nil {
		exePath = exe
	}

	log.Printf("Installing update %s to %s", m.latestRelease.Version(), exePath)

	// Perform the update
	// UpdateTo logic is wrapped in m.updater.UpdateTo usually or we use the lower level method
	// The library provides `UpdateTo(ctx, release, cmdPath)` in newer versions or similar.
	
	// Looking at creativeprojects/go-selfupdate API:
	// It has `UpdateTo(ctx context.Context, latest *Release, cmdPath string) error`
	
	if err := m.updater.UpdateTo(ctx, m.latestRelease, exePath); err != nil {
		return fmt.Errorf("failed to update binary: %w", err)
	}

	log.Println("Update verified and installed successfully")
	return nil
}

// GetLatestVersion returns the cached latest version string or empty
func (m *Manager) GetLatestVersion() string {
	if m.latestRelease != nil {
		return m.latestRelease.Version()
	}
	return ""
}
