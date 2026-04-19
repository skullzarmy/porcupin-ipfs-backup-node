package updater

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// InstallLatest downloads and applies the cached latest update.
// On macOS, replaces the entire .app bundle to preserve code signature integrity.
// On other platforms, replaces just the binary via go-selfupdate.
func (m *Manager) InstallLatest(ctx context.Context) error {
	if m.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	if m.latestRelease == nil {
		return fmt.Errorf("no update available to install")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err := filepath.EvalSymlinks(exe)
	if err != nil {
		exePath = exe
	}

	slog.Info("Installing update", "version", m.latestRelease.Version(), "path", exePath)

	// On macOS, replacing just the inner binary breaks the .app bundle's
	// code signature. Download and swap the entire bundle instead.
	if runtime.GOOS == "darwin" {
		if bundle := FindAppBundle(exePath); bundle != "" {
			return m.installMacOSBundle(ctx, bundle)
		}
	}

	// Windows/Linux: binary replacement via go-selfupdate
	if err := m.updater.UpdateTo(ctx, m.latestRelease, exePath); err != nil {
		return fmt.Errorf("failed to update binary: %w", err)
	}

	slog.Info("Update installed successfully")
	return nil
}

// installMacOSBundle downloads the full .app bundle from the release zip and
// atomically swaps it with the current bundle. This preserves the CI-built
// code signature — no local re-signing needed.
func (m *Manager) installMacOSBundle(ctx context.Context, bundlePath string) error {
	version := m.latestRelease.Version()

	// go-selfupdate matches assets by OS/arch pattern, which picks the headless
	// server binary instead of the desktop .app zip. Query the GitHub Releases
	// API directly to find the correct assets by name.
	const macOSAsset = "porcupin-macos.zip"
	const checksumAsset = "checksums.txt"

	assets, err := findReleaseAssetURLs(ctx, version, macOSAsset, checksumAsset)
	if err != nil {
		return fmt.Errorf("find release assets: %w", err)
	}
	assetURL := assets[macOSAsset]
	checksumURL := assets[checksumAsset]

	slog.Info("Downloading macOS bundle update",
		"version", version,
		"bundle", bundlePath,
		"asset", macOSAsset)

	// Fetch and parse expected checksum
	expectedHash, err := fetchExpectedChecksum(ctx, checksumURL, macOSAsset)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}

	// Download zip to temp file
	tmpZip, err := os.CreateTemp("", "porcupin-update-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpZipPath := tmpZip.Name()
	defer os.Remove(tmpZipPath)

	if err := downloadFile(ctx, assetURL, tmpZip); err != nil {
		tmpZip.Close()
		return fmt.Errorf("download asset: %w", err)
	}
	tmpZip.Close()

	// Verify SHA256 checksum
	actualHash, err := sha256File(tmpZipPath)
	if err != nil {
		return fmt.Errorf("calculate checksum: %w", err)
	}
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	slog.Info("Checksum verified", "sha256", actualHash)

	// Extract zip to a temp directory on the same volume so os.Rename is atomic
	bundleParent := filepath.Dir(bundlePath)
	tmpDir, err := os.MkdirTemp(bundleParent, ".porcupin-update-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractZip(tmpZipPath, tmpDir); err != nil {
		return fmt.Errorf("extract update: %w", err)
	}

	// Find the .app bundle in the extracted contents
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("read extracted dir: %w", err)
	}
	var newBundleSrc string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			newBundleSrc = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if newBundleSrc == "" {
		return fmt.Errorf("no .app bundle found in downloaded archive")
	}

	// Atomic swap: current → .old, new → current, remove .old
	oldBundlePath := bundlePath + ".old"
	os.RemoveAll(oldBundlePath)

	if err := os.Rename(bundlePath, oldBundlePath); err != nil {
		return fmt.Errorf("move current bundle aside: %w", err)
	}

	if err := os.Rename(newBundleSrc, bundlePath); err != nil {
		if restoreErr := os.Rename(oldBundlePath, bundlePath); restoreErr != nil {
			slog.Error("Failed to restore old bundle after swap failure", "error", restoreErr)
		}
		return fmt.Errorf("move new bundle into place: %w", err)
	}

	os.RemoveAll(oldBundlePath)

	// Clear quarantine/provenance attributes so Gatekeeper doesn't block it
	xattr := exec.Command("xattr", "-cr", bundlePath)
	if output, err := xattr.CombinedOutput(); err != nil {
		slog.Warn("Failed to clear extended attributes", "error", err, "output", string(output))
	}

	slog.Info("macOS bundle update installed successfully", "path", bundlePath)
	return nil
}

// fetchExpectedChecksum downloads checksums.txt from the release and returns
// the SHA256 hash for the given filename.
func fetchExpectedChecksum(ctx context.Context, checksumURL, filename string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Porcupin-Updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums download returned HTTP %d", resp.StatusCode)
	}

	// Format: "<hash>  <filename>" (sha256sum output)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 2 && parts[1] == filename {
			return parts[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}

	return "", fmt.Errorf("no checksum found for %s", filename)
}

// releaseAssetInfo is a partial GitHub release asset used for JSON decoding.
type releaseAssetInfo struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// releaseInfo is a partial GitHub release used for JSON decoding.
type releaseInfo struct {
	Assets []releaseAssetInfo `json:"assets"`
}

// findReleaseAssetURLs queries the GitHub Releases API for the given version
// and returns a map of asset name → browser_download_url for each requested name.
func findReleaseAssetURLs(ctx context.Context, version string, names ...string) (map[string]string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/v%s", Repository, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Porcupin-Updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d for release v%s", resp.StatusCode, version)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release JSON: %w", err)
	}

	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}

	result := make(map[string]string, len(names))
	for _, asset := range release.Assets {
		if wanted[asset.Name] {
			result[asset.Name] = asset.BrowserDownloadURL
		}
	}

	for _, n := range names {
		if result[n] == "" {
			return nil, fmt.Errorf("asset %q not found in release v%s", n, version)
		}
	}

	return result, nil
}

// downloadFile downloads a URL to the given writer.
func downloadFile(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Porcupin-Updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}

// sha256File returns the hex-encoded SHA256 hash of a file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractZip extracts a zip archive to the destination directory, preserving
// file permissions and symlinks.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		// Prevent zip slip path traversal
		rel, err := filepath.Rel(destDir, filepath.Clean(target))
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("illegal path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, f.Mode()|0755)
			continue
		}

		// Symlinks: the zip entry content is the link target
		if f.Mode()&os.ModeSymlink != 0 {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open symlink entry %s: %w", f.Name, err)
			}
			linkTarget, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("read symlink %s: %w", f.Name, err)
			}
			os.MkdirAll(filepath.Dir(target), 0755)
			if err := os.Symlink(string(linkTarget), target); err != nil {
				return fmt.Errorf("create symlink %s: %w", f.Name, err)
			}
			continue
		}

		// Regular file
		os.MkdirAll(filepath.Dir(target), 0755)
		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("create %s: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if copyErr != nil {
			return fmt.Errorf("extract %s: %w", f.Name, copyErr)
		}
	}

	return nil
}

// FindAppBundle walks up from an executable path to find the enclosing .app bundle.
func FindAppBundle(exePath string) string {
	dir := exePath
	for dir != "/" && dir != "." {
		dir = filepath.Dir(dir)
		if strings.HasSuffix(dir, ".app") {
			return dir
		}
	}
	return ""
}

// GetLatestVersion returns the cached latest version string or empty
func (m *Manager) GetLatestVersion() string {
	if m.latestRelease != nil {
		return m.latestRelease.Version()
	}
	return ""
}
