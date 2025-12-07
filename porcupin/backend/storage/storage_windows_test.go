//go:build windows
// +build windows

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// WINDOWS-SPECIFIC STORAGE TESTS
// These tests verify Windows storage functionality
// =============================================================================

func TestDetectStorageType_Windows_CDrive(t *testing.T) {
	// C: drive should be local
	st, err := DetectStorageType("C:\\Users\\test")
	if err != nil {
		t.Fatalf("DetectStorageType failed: %v", err)
	}
	if st != StorageTypeLocal {
		t.Errorf("C:\\ should be Local, got %v", st)
	}
}

func TestDetectStorageType_Windows_UNCPath(t *testing.T) {
	// UNC paths should be network
	st, err := DetectStorageType("\\\\server\\share\\folder")
	if err != nil {
		t.Fatalf("DetectStorageType failed: %v", err)
	}
	if st != StorageTypeNetwork {
		t.Errorf("UNC path should be Network, got %v", st)
	}
}

func TestGetStorageInfo_Windows_HomeDrive(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Could not get home dir")
	}

	info, err := GetStorageInfo(homeDir)
	if err != nil {
		t.Fatalf("GetStorageInfo failed: %v", err)
	}

	if info.Type != StorageTypeLocal {
		t.Errorf("Home directory should be Local storage, got %v", info.Type)
	}
	if info.TotalBytes <= 0 {
		t.Error("TotalBytes should be > 0")
	}
	if info.FreeBytes <= 0 {
		t.Error("FreeBytes should be > 0")
	}
}

func TestListAvailableLocations_Windows(t *testing.T) {
	locations, err := ListAvailableLocations()
	if err != nil {
		t.Fatalf("ListAvailableLocations failed: %v", err)
	}

	// Should at least have home directory option
	if len(locations) < 1 {
		t.Error("Expected at least one storage location (home directory)")
	}

	// First should be default
	found := false
	for _, loc := range locations {
		if loc.Label == "Default (Home Directory)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Default (Home Directory) location not found")
	}
}

func TestGetDeviceID_Windows(t *testing.T) {
	// Test getting device ID for temp directory
	tempDir := os.TempDir()
	id, err := getDeviceID(tempDir)
	if err != nil {
		t.Fatalf("getDeviceID failed: %v", err)
	}
	// Device ID should be the drive letter (e.g., 'C' = 67)
	if id == 0 {
		t.Error("Device ID should not be 0")
	}
}

func TestIsNetworkMount_Windows_LocalPath(t *testing.T) {
	// Local path should not be network
	if isNetworkMount("C:\\Windows") {
		t.Error("C:\\Windows should not be a network mount")
	}
}

func TestIsNetworkMount_Windows_UNCPath(t *testing.T) {
	// UNC paths are always network
	if !isNetworkMount("\\\\server\\share") {
		t.Error("UNC path should be detected as network mount")
	}
}

func TestGenerateLabelWindows_LocalDrive(t *testing.T) {
	label := generateLabelWindows("C:\\Users\\test", StorageTypeLocal)
	// Should contain drive letter or "Local Storage"
	if label == "" {
		t.Error("Label should not be empty")
	}
}

func TestGenerateLabelWindows_ExternalDrive(t *testing.T) {
	label := generateLabelWindows("D:\\Data", StorageTypeExternal)
	// Should contain (External) suffix
	if label == "" {
		t.Error("Label should not be empty")
	}
}

func TestExpandPath_Windows(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // Expected substring in output
	}{
		{"tilde expansion", "~\\Documents", "Users"},
		{"absolute unchanged", "C:\\Windows", "C:\\Windows"},
		{"relative unchanged", "relative\\path", "relative\\path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandPath(tt.input)
			if err != nil {
				t.Fatalf("ExpandPath failed: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
			// For relative paths, just check it returns something
			if tt.input[0] != '~' && !filepath.IsAbs(tt.input) {
				if result == "" {
					t.Error("ExpandPath returned empty for relative path")
				}
			}
		})
	}
}

func TestSameDevice_Windows_SameDrive(t *testing.T) {
	// Two paths on same drive should be same device
	same, err := SameDevice("C:\\Windows", "C:\\Users")
	if err != nil {
		t.Fatalf("SameDevice failed: %v", err)
	}
	if !same {
		t.Error("Paths on same drive should be same device")
	}
}

func TestSameDevice_Windows_DifferentDrives(t *testing.T) {
	// Only test if D: drive exists
	if _, err := os.Stat("D:\\"); os.IsNotExist(err) {
		t.Skip("D: drive does not exist")
	}

	same, err := SameDevice("C:\\Windows", "D:\\")
	if err != nil {
		t.Fatalf("SameDevice failed: %v", err)
	}
	if same {
		t.Error("Paths on different drives should not be same device")
	}
}
