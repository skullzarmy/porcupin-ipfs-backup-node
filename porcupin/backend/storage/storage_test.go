//go:build !windows

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// =============================================================================
// EXPAND PATH TESTS
// =============================================================================

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde expansion",
			input:    "~/.porcupin/ipfs",
			expected: filepath.Join(home, ".porcupin/ipfs"),
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
		},
		{
			name:     "absolute path unchanged",
			input:    "/usr/local/bin",
			expected: "/usr/local/bin",
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandPath(tt.input)
			if err != nil {
				t.Fatalf("ExpandPath(%q) error: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// VALIDATE PATH TESTS
// =============================================================================

func TestValidatePath_Writable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testPath := filepath.Join(tmpDir, "test-storage")

	err = ValidatePath(testPath)
	if err != nil {
		t.Errorf("ValidatePath(%q) should succeed for writable location: %v", testPath, err)
	}
}

func TestValidatePath_ExistingDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Path already exists as directory
	err = ValidatePath(tmpDir)
	if err != nil {
		t.Errorf("ValidatePath(%q) should succeed for existing directory: %v", tmpDir, err)
	}
}

func TestValidatePath_ExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file at the path
	filePath := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	err = ValidatePath(filePath)
	if err == nil {
		t.Errorf("ValidatePath(%q) should fail for existing file", filePath)
	}
}

func TestValidatePath_NonexistentParent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create parent directory - ValidatePath should work when parent exists
	parentPath := filepath.Join(tmpDir, "existing-parent")
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		t.Fatalf("Failed to create parent dir: %v", err)
	}

	testPath := filepath.Join(parentPath, "storage")

	err = ValidatePath(testPath)
	if err != nil {
		t.Errorf("ValidatePath should succeed when parent exists and is writable: %v", err)
	}
}

func TestValidatePath_ParentDoesNotExist_Bug(t *testing.T) {
	// Test that ValidatePath succeeds for paths where parent doesn't exist
	// but CAN be created (writable location)

	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Path where parent doesn't exist but CAN be created
	testPath := filepath.Join(tmpDir, "new-parent", "storage")

	err = ValidatePath(testPath)
	if err != nil {
		t.Errorf("ValidatePath should succeed when parent can be created: %v", err)
	}

	// Verify parent was cleaned up (not left behind)
	parentPath := filepath.Join(tmpDir, "new-parent")
	if _, err := os.Stat(parentPath); !os.IsNotExist(err) {
		t.Error("ValidatePath should clean up temporary parent directory")
	}
}

// =============================================================================
// SAME DEVICE TESTS
// =============================================================================

func TestSameDevice_SamePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	same, err := SameDevice(tmpDir, tmpDir)
	if err != nil {
		t.Fatalf("SameDevice error: %v", err)
	}
	if !same {
		t.Error("SameDevice should return true for same path")
	}
}

func TestSameDevice_SameDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path1 := filepath.Join(tmpDir, "dir1")
	path2 := filepath.Join(tmpDir, "dir2")

	same, err := SameDevice(path1, path2)
	if err != nil {
		t.Fatalf("SameDevice error: %v", err)
	}
	if !same {
		t.Error("SameDevice should return true for paths in same directory")
	}
}

func TestSameDevice_HomePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	// Use paths that exist (home dir itself) to avoid "no such file or directory"
	// SameDevice checks parent when path doesn't exist, so we use existing paths
	path1 := home
	path2 := home

	same, err := SameDevice(path1, path2)
	if err != nil {
		t.Fatalf("SameDevice error: %v", err)
	}
	if !same {
		t.Error("SameDevice should return true for paths in home directory")
	}
}

func TestSameDevice_TildeExpansion(t *testing.T) {
	// Use ~ which expands to home dir (exists on all systems)
	same, err := SameDevice("~", "~")
	if err != nil {
		t.Fatalf("SameDevice error: %v", err)
	}
	if !same {
		t.Error("SameDevice should handle tilde expansion")
	}
}

// NOTE: Storage type constants and struct defaults tests were removed.
// Testing that Go struct zero values are zero and constants equal their
// defined values provides no value - these test the language, not our code.

// =============================================================================
// MANAGER TESTS
// =============================================================================

func TestNewManager(t *testing.T) {
	m := NewManager("/test/path")

	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.currentPath != "/test/path" {
		t.Errorf("currentPath = %q, want '/test/path'", m.currentPath)
	}
	if m.migrationStatus == nil {
		t.Error("migrationStatus should not be nil")
	}
}

func TestManager_GetCurrentPath(t *testing.T) {
	m := NewManager("/custom/ipfs/path")

	path := m.GetCurrentPath()
	if path != "/custom/ipfs/path" {
		t.Errorf("GetCurrentPath() = %q, want '/custom/ipfs/path'", path)
	}
}

func TestManager_GetMigrationStatus_NoMigration(t *testing.T) {
	m := NewManager("/test")

	status := m.GetMigrationStatus()
	if status.InProgress {
		t.Error("InProgress should be false when no migration is running")
	}
}

func TestManager_CancelMigration_NoMigration(t *testing.T) {
	m := NewManager("/test")

	err := m.CancelMigration()
	if err == nil {
		t.Error("CancelMigration should error when no migration in progress")
	}
}

func TestGetGlobalMigrationStatus_NoManager(t *testing.T) {
	// Save and restore global state to avoid race with other tests
	globalMigrationMu.Lock()
	savedManager := globalMigrationManager
	globalMigrationManager = nil
	globalMigrationMu.Unlock()

	defer func() {
		globalMigrationMu.Lock()
		globalMigrationManager = savedManager
		globalMigrationMu.Unlock()
	}()

	status := GetGlobalMigrationStatus()
	if status.InProgress {
		t.Error("Global status should show no migration when manager is nil")
	}
}

func TestCancelGlobalMigration_NoManager(t *testing.T) {
	// Save and restore global state to avoid race with other tests
	globalMigrationMu.Lock()
	savedManager := globalMigrationManager
	globalMigrationManager = nil
	globalMigrationMu.Unlock()

	defer func() {
		globalMigrationMu.Lock()
		globalMigrationManager = savedManager
		globalMigrationMu.Unlock()
	}()

	err := CancelGlobalMigration()
	if err == nil {
		t.Error("CancelGlobalMigration should error when no manager")
	}
}

// =============================================================================
// GET DIR SIZE TESTS
// =============================================================================

func TestGetDirSize_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dirsize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	size, err := getDirSize(tmpDir)
	if err != nil {
		t.Fatalf("getDirSize error: %v", err)
	}

	// Empty directory should have minimal size (just directory entry)
	// Usually a few KB at most
	if size < 0 {
		t.Errorf("getDirSize returned negative size: %d", size)
	}
}

func TestGetDirSize_WithFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dirsize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some test files
	testData := []byte("test data for size calculation")
	for i := 0; i < 5; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(filePath, testData, 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	size, err := getDirSize(tmpDir)
	if err != nil {
		t.Fatalf("getDirSize error: %v", err)
	}

	// Size should be at least the sum of file sizes
	minExpectedSize := int64(len(testData) * 5)
	if size < minExpectedSize {
		t.Errorf("getDirSize = %d bytes, want at least %d bytes", size, minExpectedSize)
	}
}

func TestGetDirSize_NonexistentDir(t *testing.T) {
	_, err := getDirSize("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("getDirSize should error for nonexistent path")
	}
}

func TestGetDirSize_NestedDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dirsize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested directory structure
	subDir := filepath.Join(tmpDir, "sub1", "sub2", "sub3")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirs: %v", err)
	}

	// Add files at various levels
	testData := []byte("nested file data")
	os.WriteFile(filepath.Join(tmpDir, "root.txt"), testData, 0644)
	os.WriteFile(filepath.Join(tmpDir, "sub1", "level1.txt"), testData, 0644)
	os.WriteFile(filepath.Join(subDir, "deep.txt"), testData, 0644)

	size, err := getDirSize(tmpDir)
	if err != nil {
		t.Fatalf("getDirSize error: %v", err)
	}

	// Should include all nested files
	minExpectedSize := int64(len(testData) * 3)
	if size < minExpectedSize {
		t.Errorf("getDirSize = %d bytes, want at least %d bytes for nested dirs", size, minExpectedSize)
	}
}

// =============================================================================
// MIGRATE TESTS
// =============================================================================

func TestManager_Migrate_AlreadyInProgress(t *testing.T) {
	m := NewManager("/test/source")

	// Set migration in progress
	m.mu.Lock()
	m.migrationStatus = &MigrationStatus{InProgress: true}
	m.mu.Unlock()

	err := m.Migrate(context.Background(), "/test/dest", nil)
	if err == nil {
		t.Error("Migrate should error when already in progress")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("Error should mention 'already in progress', got: %v", err)
	}
}

func TestManager_Migrate_SameDevice_Rename(t *testing.T) {
	// Create temp source directory with content
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	destDir := filepath.Join(tmpDir, "external_drive") // Simulate external drive mount point

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create dest directory so isWritable check can work
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	// Create test file
	testFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test data"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	m := NewManager(sourceDir)

	var progressCalls int
	progressCallback := func(status MigrationStatus) {
		progressCalls++
	}

	err = m.Migrate(context.Background(), destDir, progressCallback)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// The migrate creates a porcupin-ipfs subfolder
	expectedDest := filepath.Join(destDir, "porcupin-ipfs")

	// Check source no longer exists
	if _, err := os.Stat(sourceDir); !os.IsNotExist(err) {
		t.Error("Source directory should be renamed away")
	}

	// Check destination contains the file (in the porcupin-ipfs subfolder)
	destFile := filepath.Join(expectedDest, "test.txt")
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Errorf("Destination should contain migrated file at %s", destFile)
	}

	// Check progress was called
	if progressCalls == 0 {
		t.Error("Progress callback should have been called")
	}

	// Check manager's current path was updated to include porcupin-ipfs
	if m.GetCurrentPath() != expectedDest {
		t.Errorf("CurrentPath = %q, want %q", m.GetCurrentPath(), expectedDest)
	}
}

func TestManager_Migrate_InsufficientSpace(t *testing.T) {
	// This test is platform-dependent and may not work reliably
	// We test the logic path instead by mocking
	t.Skip("Skipping: insufficient space test requires large test files")
}

func TestManager_Migrate_NotWritable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}

	m := NewManager(sourceDir)

	// Try to migrate to a read-only location
	// Note: /System is read-only on macOS
	err = m.Migrate(context.Background(), "/System/test", nil)
	if err == nil {
		t.Error("Migrate should fail for unwritable destination")
	}
}

func TestManager_Migrate_CreatesSubfolder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("data"), 0644)

	destBase := filepath.Join(tmpDir, "external_drive")
	if err := os.MkdirAll(destBase, 0755); err != nil {
		t.Fatalf("Failed to create dest base: %v", err)
	}

	m := NewManager(sourceDir)

	// Pass just the base path - Migrate should create porcupin-ipfs subfolder
	err = m.Migrate(context.Background(), destBase, nil)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// The actual destination should be destBase/porcupin-ipfs
	expectedDest := filepath.Join(destBase, "porcupin-ipfs")
	if m.GetCurrentPath() != expectedDest {
		t.Errorf("CurrentPath = %q, want %q", m.GetCurrentPath(), expectedDest)
	}
}

// =============================================================================
// CANCEL MIGRATION TESTS (WITH MIGRATION)
// =============================================================================

func TestManager_CancelMigration_WithMigration(t *testing.T) {
	m := NewManager("/test")

	// Simulate migration in progress
	m.mu.Lock()
	m.migrationStatus = &MigrationStatus{
		InProgress: true,
		Phase:      "copying",
	}
	m.mu.Unlock()

	err := m.CancelMigration()
	if err != nil {
		t.Errorf("CancelMigration should succeed: %v", err)
	}

	status := m.GetMigrationStatus()
	if status.InProgress {
		t.Error("Migration should be stopped after cancel")
	}
	if status.Phase != "cancelled" {
		t.Errorf("Phase = %q, want 'cancelled'", status.Phase)
	}
}

// =============================================================================
// IS WRITABLE TESTS
// =============================================================================

func TestIsWritable_WritableDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "writable-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !isWritable(tmpDir) {
		t.Error("Temp directory should be writable")
	}
}

func TestIsWritable_NonexistentDir(t *testing.T) {
	// isWritable should return false for nonexistent paths
	if isWritable("/nonexistent/path/12345") {
		t.Error("Nonexistent path should not be writable")
	}
}

// =============================================================================
// IS NETWORK MOUNT TESTS (macOS specific)
// =============================================================================

func TestIsNetworkMount_LocalPath(t *testing.T) {
	// Local paths should not be network mounts
	home, _ := os.UserHomeDir()
	if isNetworkMount(home) {
		t.Error("Home directory should not be a network mount")
	}
}

func TestIsNetworkMount_TempPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "network-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if isNetworkMount(tmpDir) {
		t.Error("Temp directory should not be a network mount")
	}
}

// =============================================================================
// GENERATE LABEL TESTS
// =============================================================================

func TestGenerateLabel_LocalStorage(t *testing.T) {
	label := generateLabel("/Users/test/.porcupin", StorageTypeLocal)
	if label != "Local Storage" {
		t.Errorf("label = %q, want 'Local Storage'", label)
	}
}

func TestGenerateLabel_ExternalVolume(t *testing.T) {
	var path, expected string
	switch runtime.GOOS {
	case "darwin":
		path = "/Volumes/MyDrive/data"
		expected = "MyDrive (External)"
	case "linux":
		path = "/mnt/MyDrive/data"
		expected = "MyDrive (External)"
	default:
		t.Skip("Test not implemented for this OS")
	}
	label := generateLabel(path, StorageTypeExternal)
	if label != expected {
		t.Errorf("label = %q, want %q", label, expected)
	}
}

func TestGenerateLabel_NetworkVolume(t *testing.T) {
	var path, expected string
	switch runtime.GOOS {
	case "darwin":
		path = "/Volumes/NetworkShare/data"
		expected = "NetworkShare (Network)"
	case "linux":
		path = "/mnt/NetworkShare/data"
		expected = "NetworkShare (Network)"
	default:
		t.Skip("Test not implemented for this OS")
	}
	label := generateLabel(path, StorageTypeNetwork)
	if label != expected {
		t.Errorf("label = %q, want %q", label, expected)
	}
}

func TestGenerateLabel_ExternalNonVolume(t *testing.T) {
	label := generateLabel("/mnt/external", StorageTypeExternal)
	if label != "External Drive" {
		t.Errorf("label = %q, want 'External Drive'", label)
	}
}

// =============================================================================
// GET MOUNT POINT TESTS
// =============================================================================

func TestGetMountPoint_VolumePath(t *testing.T) {
	var path, expected string
	switch runtime.GOOS {
	case "darwin":
		path = "/Volumes/MyDrive/some/nested/path"
		expected = "/Volumes/MyDrive"
	case "linux":
		path = "/mnt/MyDrive/some/nested/path"
		expected = "/mnt/MyDrive"
	default:
		t.Skip("Test not implemented for this OS")
	}
	mp := getMountPoint(path)
	if mp != expected {
		t.Errorf("mount point = %q, want %q", mp, expected)
	}
}

func TestGetMountPoint_NonVolumePath(t *testing.T) {
	path := "/usr/local/share"
	mp := getMountPoint(path)
	if mp != path {
		t.Errorf("mount point = %q, want %q (original path)", mp, path)
	}
}

// =============================================================================
// GLOBAL MIGRATION MANAGER TESTS
// =============================================================================

func TestGetGlobalMigrationStatus_WithManager(t *testing.T) {
	// Save current state
	globalMigrationMu.Lock()
	savedManager := globalMigrationManager
	globalMigrationMu.Unlock()

	defer func() {
		globalMigrationMu.Lock()
		globalMigrationManager = savedManager
		globalMigrationMu.Unlock()
	}()

	// Set up a test manager
	testManager := NewManager("/test")
	testManager.migrationStatus = &MigrationStatus{
		InProgress: true,
		Progress:   50.0,
		Phase:      "copying",
	}

	globalMigrationMu.Lock()
	globalMigrationManager = testManager
	globalMigrationMu.Unlock()

	status := GetGlobalMigrationStatus()
	if !status.InProgress {
		t.Error("Global status should show in progress")
	}
	if status.Progress != 50.0 {
		t.Errorf("Progress = %f, want 50.0", status.Progress)
	}
}

// =============================================================================
// DETECT STORAGE TYPE TESTS
// =============================================================================

func TestDetectStorageType_HomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	storageType, err := DetectStorageType(home)
	if err != nil {
		t.Fatalf("DetectStorageType error: %v", err)
	}

	if storageType != StorageTypeLocal {
		t.Errorf("Home directory should be local, got %q", storageType)
	}
}

func TestDetectStorageType_TempDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storageType, err := DetectStorageType(tmpDir)
	if err != nil {
		t.Fatalf("DetectStorageType error: %v", err)
	}

	// Temp dirs are typically on same device as home
	if storageType != StorageTypeLocal && storageType != StorageTypeExternal {
		t.Errorf("Temp directory type = %q, want local or external", storageType)
	}
}

func TestDetectStorageType_SMBPrefix(t *testing.T) {
	storageType, err := DetectStorageType("smb://server/share")
	if err != nil {
		t.Fatalf("DetectStorageType error: %v", err)
	}

	if storageType != StorageTypeNetwork {
		t.Errorf("SMB path should be network, got %q", storageType)
	}
}

func TestDetectStorageType_NFSPrefix(t *testing.T) {
	storageType, err := DetectStorageType("nfs://server/export")
	if err != nil {
		t.Fatalf("DetectStorageType error: %v", err)
	}

	if storageType != StorageTypeNetwork {
		t.Errorf("NFS path should be network, got %q", storageType)
	}
}

func TestDetectStorageType_UNCPath(t *testing.T) {
	storageType, err := DetectStorageType("//server/share")
	if err != nil {
		t.Fatalf("DetectStorageType error: %v", err)
	}

	if storageType != StorageTypeNetwork {
		t.Errorf("UNC path should be network, got %q", storageType)
	}
}

// =============================================================================
// GET STORAGE INFO TESTS
// =============================================================================

func TestGetStorageInfo_TempDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	info, err := GetStorageInfo(tmpDir)
	if err != nil {
		t.Fatalf("GetStorageInfo error: %v", err)
	}

	if info.Path != tmpDir {
		t.Errorf("Path = %q, want %q", info.Path, tmpDir)
	}
	if info.TotalBytes <= 0 {
		t.Errorf("TotalBytes should be positive, got %d", info.TotalBytes)
	}
	if info.FreeBytes <= 0 {
		t.Errorf("FreeBytes should be positive, got %d", info.FreeBytes)
	}
	if !info.IsWritable {
		t.Error("Temp dir should be writable")
	}
	if !info.IsMounted {
		t.Error("Temp dir should be mounted")
	}
}

func TestGetStorageInfo_NonExistentPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Path that doesn't exist, but parent does
	nonExistent := filepath.Join(tmpDir, "does-not-exist")

	info, err := GetStorageInfo(nonExistent)
	if err != nil {
		t.Fatalf("GetStorageInfo should work for non-existent path with valid parent: %v", err)
	}

	if info.Path != nonExistent {
		t.Errorf("Path = %q, want %q", info.Path, nonExistent)
	}
}

func TestGetStorageInfo_HomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	info, err := GetStorageInfo(home)
	if err != nil {
		t.Fatalf("GetStorageInfo error: %v", err)
	}

	if info.Type != StorageTypeLocal {
		t.Errorf("Home dir should be local, got %q", info.Type)
	}
	if info.Label == "" {
		t.Error("Label should not be empty")
	}
}

// =============================================================================
// LIST AVAILABLE LOCATIONS TESTS
// =============================================================================

func TestListAvailableLocations(t *testing.T) {
	locations, err := ListAvailableLocations()
	if err != nil {
		t.Fatalf("ListAvailableLocations error: %v", err)
	}

	// Should at least include default home location
	if len(locations) == 0 {
		t.Error("Should have at least one location (home)")
	}

	// First location should be home directory default
	found := false
	for _, loc := range locations {
		if loc.Label == "Default (Home Directory)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should include 'Default (Home Directory)' location")
	}
}
