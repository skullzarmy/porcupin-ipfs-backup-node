package updater

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

const (
	// Repository is the GitHub repository to check for updates
	Repository = "skullzarmy/porcupin-ipfs-backup-node"
)

// Release interface abstracts the selfupdate.Release struct
type Release interface {
	Version() string
	LessOrEqual(version string) bool
	// Add other methods/fields as needed via getters if strictly necessary
	// logic relying on direct field access (ReleaseNotes, etc) will need accessors or type assertion
	// For now, we expose what Manager uses:
	GetReleaseNotes() string
	GetPublishedAt() time.Time
	GetAssetURL() string
}

// selfupdateReleaseAdapter wraps *selfupdate.Release to satisfy Release interface
type selfupdateReleaseAdapter struct {
	*selfupdate.Release
}

func (r *selfupdateReleaseAdapter) GetReleaseNotes() string { return r.ReleaseNotes }
func (r *selfupdateReleaseAdapter) GetPublishedAt() time.Time { return r.PublishedAt }
func (r *selfupdateReleaseAdapter) GetAssetURL() string { return r.AssetURL }


// Updater interface allows mocking the selfupdate.Updater
type Updater interface {
	DetectLatest(ctx context.Context, repository selfupdate.Repository) (Release, bool, error)
	UpdateTo(ctx context.Context, release Release, cmdPath string) error
}

// RealUpdater adapts the concrete selfupdate.Updater to our interface
type RealUpdater struct {
	*selfupdate.Updater
}

func (u *RealUpdater) DetectLatest(ctx context.Context, repository selfupdate.Repository) (Release, bool, error) {
	rel, found, err := u.Updater.DetectLatest(ctx, repository)
	if rel != nil {
		return &selfupdateReleaseAdapter{rel}, found, err
	}
	return nil, found, err
}

func (u *RealUpdater) UpdateTo(ctx context.Context, release Release, cmdPath string) error {
	// Type assert back to concrete type for the library
	adapter, ok := release.(*selfupdateReleaseAdapter)
	if !ok {
		return fmt.Errorf("invalid release type")
	}
	return u.Updater.UpdateTo(ctx, adapter.Release, cmdPath)
}

// Manager handles the update process
type Manager struct {
	updater      Updater
	currentVer   string
	latestRelease Release
}

// NewManager creates a new update manager
func NewManager(currentVersion string) (*Manager, error) {
	// Configure updater
	up, err := selfupdate.NewUpdater(selfupdate.Config{
		Validator: &selfupdate.ChecksumValidator{
			UniqueFilename: "checksums.txt",
		},
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to create updater: %w", err)
	}

	return &Manager{
		updater:    &RealUpdater{up},
		currentVer: currentVersion,
	}, nil
}

// CheckForUpdates checks GitHub for a newer version
func (m *Manager) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	if m.updater == nil {
		return nil, fmt.Errorf("updater not initialized")
	}

	slog.Info("Checking for updates", "current_version", m.currentVer)
	
	latest, found, err := m.updater.DetectLatest(ctx, selfupdate.ParseSlug(Repository))
	if err != nil {
		return nil, fmt.Errorf("failed to detect update: %w", err)
	}

	info := &UpdateInfo{
		CurrentVer: m.currentVer,
		Available:  false,
	}

	if !found {
		slog.Info("No updates found")
		return info, nil
	}

	// Compare versions
	if latest.LessOrEqual(m.currentVer) {
		slog.Info("Latest version is not newer", "latest", latest.Version(), "current", m.currentVer)
		return info, nil
	}

	slog.Info("New version found", "version", latest.Version())
	
	// Cache the release for installation step
	m.latestRelease = latest

	// Populate info
	info.Available = true
	info.Version = latest.Version()
	info.ReleaseNotes = latest.GetReleaseNotes()
	info.PubDate = latest.GetPublishedAt()
	info.AssetURL = latest.GetAssetURL()
	
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

	slog.Info("Installing update", "version", m.latestRelease.Version(), "path", exePath)

	if err := m.updater.UpdateTo(ctx, m.latestRelease, exePath); err != nil {
		return fmt.Errorf("failed to update binary: %w", err)
	}

	slog.Info("Update verified and installed successfully")
	return nil
}

// GetLatestVersion returns the cached latest version string or empty
func (m *Manager) GetLatestVersion() string {
	if m.latestRelease != nil {
		return m.latestRelease.Version()
	}
	return ""
}
