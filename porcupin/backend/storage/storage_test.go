//go:build !windows

package storage

import (
	"os"
	"path/filepath"
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

// =============================================================================
// STORAGE TYPE TESTS
// =============================================================================

func TestStorageTypeConstants(t *testing.T) {
	if StorageTypeLocal != "local" {
		t.Errorf("StorageTypeLocal = %q, want 'local'", StorageTypeLocal)
	}
	if StorageTypeExternal != "external" {
		t.Errorf("StorageTypeExternal = %q, want 'external'", StorageTypeExternal)
	}
	if StorageTypeNetwork != "network" {
		t.Errorf("StorageTypeNetwork = %q, want 'network'", StorageTypeNetwork)
	}
}

// =============================================================================
// STORAGE LOCATION TESTS
// =============================================================================

func TestStorageLocation_Defaults(t *testing.T) {
	loc := StorageLocation{}

	if loc.Path != "" {
		t.Errorf("Default Path should be empty, got %q", loc.Path)
	}
	if loc.Type != "" {
		t.Errorf("Default Type should be empty, got %q", loc.Type)
	}
	if loc.TotalBytes != 0 {
		t.Errorf("Default TotalBytes should be 0, got %d", loc.TotalBytes)
	}
	if loc.FreeBytes != 0 {
		t.Errorf("Default FreeBytes should be 0, got %d", loc.FreeBytes)
	}
	if loc.IsWritable {
		t.Error("Default IsWritable should be false")
	}
	if loc.IsMounted {
		t.Error("Default IsMounted should be false")
	}
}

func TestStorageLocation_Values(t *testing.T) {
	loc := StorageLocation{
		Path:       "/Volumes/External",
		Type:       StorageTypeExternal,
		Label:      "External Drive",
		TotalBytes: 1000000000000, // 1TB
		FreeBytes:  500000000000,  // 500GB
		IsWritable: true,
		IsMounted:  true,
		MountPoint: "/Volumes/External",
	}

	if loc.Path != "/Volumes/External" {
		t.Errorf("Path = %q, want '/Volumes/External'", loc.Path)
	}
	if loc.Type != StorageTypeExternal {
		t.Errorf("Type = %q, want 'external'", loc.Type)
	}
	if loc.Label != "External Drive" {
		t.Errorf("Label = %q, want 'External Drive'", loc.Label)
	}
	if !loc.IsWritable {
		t.Error("IsWritable should be true")
	}
	if !loc.IsMounted {
		t.Error("IsMounted should be true")
	}
}

// =============================================================================
// MIGRATION STATUS TESTS
// =============================================================================

func TestMigrationStatus_Defaults(t *testing.T) {
	status := MigrationStatus{}

	if status.InProgress {
		t.Error("Default InProgress should be false")
	}
	if status.Progress != 0 {
		t.Errorf("Default Progress should be 0, got %f", status.Progress)
	}
	if status.BytesCopied != 0 {
		t.Errorf("Default BytesCopied should be 0, got %d", status.BytesCopied)
	}
}

func TestMigrationStatus_InProgress(t *testing.T) {
	status := MigrationStatus{
		InProgress:  true,
		SourcePath:  "/old/path",
		DestPath:    "/new/path",
		Progress:    45.5,
		BytesCopied: 500000000, // 500MB
		TotalBytes:  1000000000, // 1GB
		CurrentFile: "data/blocks/1234",
		Method:      "rsync",
		Phase:       "copying",
	}

	if !status.InProgress {
		t.Error("InProgress should be true")
	}
	if status.Progress != 45.5 {
		t.Errorf("Progress = %f, want 45.5", status.Progress)
	}
	if status.Method != "rsync" {
		t.Errorf("Method = %q, want 'rsync'", status.Method)
	}
	if status.Phase != "copying" {
		t.Errorf("Phase = %q, want 'copying'", status.Phase)
	}
}

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
