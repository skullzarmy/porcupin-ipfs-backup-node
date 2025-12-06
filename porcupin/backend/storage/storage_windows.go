//go:build windows
// +build windows

package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW   = kernel32.NewProc("GetDiskFreeSpaceExW")
	getVolumeInformationW = kernel32.NewProc("GetVolumeInformationW")
	getDriveTypeW         = kernel32.NewProc("GetDriveTypeW")
)

const (
	DRIVE_UNKNOWN     = 0
	DRIVE_NO_ROOT_DIR = 1
	DRIVE_REMOVABLE   = 2
	DRIVE_FIXED       = 3
	DRIVE_REMOTE      = 4
	DRIVE_CDROM       = 5
	DRIVE_RAMDISK     = 6
)

// getDeviceID returns a pseudo device ID for Windows (drive letter hash)
func getDeviceID(path string) (uint64, error) {
	// Expand path
	path, err := ExpandPath(path)
	if err != nil {
		return 0, err
	}

	// Get the volume root (e.g., "C:\")
	absPath, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}

	root := filepath.VolumeName(absPath) + "\\"

	// Use drive letter as pseudo device ID
	if len(root) > 0 {
		return uint64(root[0]), nil
	}
	return 0, nil
}

// isNetworkMount checks if a path is on a network mount (Windows)
func isNetworkMount(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// UNC paths are always network
	if len(absPath) >= 2 && absPath[0] == '\\' && absPath[1] == '\\' {
		return true
	}

	root := filepath.VolumeName(absPath) + "\\"
	rootPtr, _ := syscall.UTF16PtrFromString(root)

	ret, _, _ := getDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))
	return ret == DRIVE_REMOTE
}

// DetectStorageType determines if a path is local, external, or network storage (Windows)
func DetectStorageType(path string) (StorageType, error) {
	// Expand path
	path, err := ExpandPath(path)
	if err != nil {
		return StorageTypeLocal, err
	}

	// UNC paths are network
	if len(path) >= 2 && path[0] == '\\' && path[1] == '\\' {
		return StorageTypeNetwork, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return StorageTypeLocal, err
	}

	root := filepath.VolumeName(absPath) + "\\"
	rootPtr, _ := syscall.UTF16PtrFromString(root)

	ret, _, _ := getDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))

	switch ret {
	case DRIVE_REMOTE:
		return StorageTypeNetwork, nil
	case DRIVE_REMOVABLE:
		return StorageTypeExternal, nil
	case DRIVE_FIXED:
		// Check if it's the same drive as home
		homeDir, _ := os.UserHomeDir()
		homeRoot := filepath.VolumeName(homeDir) + "\\"
		if root != homeRoot {
			return StorageTypeExternal, nil
		}
		return StorageTypeLocal, nil
	default:
		return StorageTypeLocal, nil
	}
}

// GetStorageInfo returns information about a storage location (Windows)
func GetStorageInfo(path string) (*StorageLocation, error) {
	// Expand path
	path, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}

	storageType, err := DetectStorageType(path)
	if err != nil {
		return nil, err
	}

	loc := &StorageLocation{
		Path: path,
		Type: storageType,
	}

	// Get disk space info
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	root := filepath.VolumeName(absPath) + "\\"
	rootPtr, _ := syscall.UTF16PtrFromString(root)

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret != 0 {
		loc.TotalBytes = int64(totalBytes)
		loc.FreeBytes = int64(freeBytesAvailable)
		loc.IsMounted = true
	}

	// Check if writable
	loc.IsWritable = isWritable(path)

	// Generate label
	loc.Label = generateLabelWindows(path, storageType)

	// Get mount point for external/network
	if storageType != StorageTypeLocal {
		loc.MountPoint = root
	}

	return loc, nil
}

// generateLabelWindows creates a human-readable label for Windows storage
func generateLabelWindows(path string, storageType StorageType) string {
	absPath, _ := filepath.Abs(path)
	root := filepath.VolumeName(absPath) + "\\"

	// Try to get volume label
	rootPtr, _ := syscall.UTF16PtrFromString(root)
	volumeName := make([]uint16, 256)

	ret, _, _ := getVolumeInformationW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&volumeName[0])),
		256,
		0, 0, 0, 0, 0,
	)

	if ret != 0 {
		name := syscall.UTF16ToString(volumeName)
		if name != "" {
			switch storageType {
			case StorageTypeExternal:
				return name + " (External)"
			case StorageTypeNetwork:
				return name + " (Network)"
			default:
				return name
			}
		}
	}

	switch storageType {
	case StorageTypeExternal:
		return root + " (External)"
	case StorageTypeNetwork:
		return "Network Storage"
	default:
		return "Local Storage (" + root + ")"
	}
}

// ListAvailableLocations returns a list of available storage locations (Windows)
func ListAvailableLocations() ([]*StorageLocation, error) {
	var locations []*StorageLocation

	// Always include home directory option
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(homeDir, ".porcupin", "ipfs")
		loc, err := GetStorageInfo(defaultPath)
		if err == nil {
			loc.Label = "Default (Home Directory)"
			locations = append(locations, loc)
		}
	}

	// Scan all drive letters
	for letter := 'C'; letter <= 'Z'; letter++ {
		root := string(letter) + ":\\"
		rootPtr, _ := syscall.UTF16PtrFromString(root)

		ret, _, _ := getDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))

		// Include removable, fixed (non-system), and network drives
		if ret == DRIVE_REMOVABLE || ret == DRIVE_REMOTE ||
			(ret == DRIVE_FIXED && string(letter) != "C") {
			loc, err := GetStorageInfo(root)
			if err == nil && loc.IsWritable && loc.IsMounted {
				suggestedPath := filepath.Join(root, "porcupin-ipfs")
				loc.Path = suggestedPath
				locations = append(locations, loc)
			}
		}
	}

	return locations, nil
}
