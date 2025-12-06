//go:build !windows
// +build !windows

package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// getDeviceID returns the device ID for a path (Unix only)
func getDeviceID(path string) (uint64, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Dev), nil
}

// isNetworkMount checks if a path is on a network mount
func isNetworkMount(path string) bool {
	// Read /proc/mounts or use mount command to check
	if runtime.GOOS == "darwin" {
		// On macOS, check if it's under /Volumes and is a network share
		out, err := exec.Command("mount").Output()
		if err != nil {
			return false
		}
		
		// Find the mount point for this path
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "smbfs") || strings.Contains(line, "nfs") || 
			   strings.Contains(line, "afpfs") || strings.Contains(line, "cifs") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					mountPoint := parts[2]
					if strings.HasPrefix(path, mountPoint) {
						return true
					}
				}
			}
		}
	}
	return false
}

// DetectStorageType determines if a path is local, external, or network storage
func DetectStorageType(path string) (StorageType, error) {
	// Expand path
	path, err := ExpandPath(path)
	if err != nil {
		return StorageTypeLocal, err
	}

	// Check if it's a network path
	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, "\\\\") {
		return StorageTypeNetwork, nil
	}

	// Check for SMB-style paths
	if strings.Contains(path, "smb://") || strings.Contains(path, "nfs://") {
		return StorageTypeNetwork, nil
	}

	// On macOS/Linux, check mount points
	homeDir, _ := os.UserHomeDir()
	homeStat, err := getDeviceID(homeDir)
	if err != nil {
		return StorageTypeLocal, nil // Assume local if we can't stat
	}

	pathStat, err := getDeviceID(path)
	if err != nil {
		// Path might not exist yet, check parent
		parentPath := filepath.Dir(path)
		pathStat, err = getDeviceID(parentPath)
		if err != nil {
			return StorageTypeLocal, nil
		}
	}

	// If on different device, it's external
	if homeStat != pathStat {
		// Check if it's a network mount
		if isNetworkMount(path) {
			return StorageTypeNetwork, nil
		}
		return StorageTypeExternal, nil
	}

	return StorageTypeLocal, nil
}

// GetStorageInfo returns information about a storage location
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
	var stat syscall.Statfs_t
	checkPath := path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		checkPath = filepath.Dir(path)
	}
	
	if err := syscall.Statfs(checkPath, &stat); err == nil {
		loc.TotalBytes = int64(stat.Blocks) * int64(stat.Bsize)
		loc.FreeBytes = int64(stat.Bavail) * int64(stat.Bsize)
		loc.IsMounted = true
	}

	// Check if writable
	loc.IsWritable = isWritable(checkPath)

	// Generate label
	loc.Label = generateLabel(path, storageType)

	// Get mount point for external/network
	if storageType != StorageTypeLocal {
		loc.MountPoint = getMountPoint(path)
	}

	return loc, nil
}

// generateLabel creates a human-readable label for a storage location
func generateLabel(path string, storageType StorageType) string {
	// Extract volume name from path on macOS
	if runtime.GOOS == "darwin" && strings.HasPrefix(path, "/Volumes/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			volumeName := parts[2]
			switch storageType {
			case StorageTypeExternal:
				return volumeName + " (External)"
			case StorageTypeNetwork:
				return volumeName + " (Network)"
			default:
				return volumeName
			}
		}
	}
	
	switch storageType {
	case StorageTypeExternal:
		return "External Drive"
	case StorageTypeNetwork:
		return "Network Drive"
	default:
		return "Local Storage"
	}
}

// getMountPoint finds the mount point for a path
func getMountPoint(path string) string {
	if runtime.GOOS == "darwin" {
		if strings.HasPrefix(path, "/Volumes/") {
			parts := strings.Split(path, "/")
			if len(parts) >= 3 {
				return "/Volumes/" + parts[2]
			}
		}
	}
	return path
}

// ListAvailableLocations returns a list of available storage locations
func ListAvailableLocations() ([]*StorageLocation, error) {
	var locations []*StorageLocation

	// Always include home directory as the default option
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(homeDir, ".porcupin", "ipfs")
		loc, err := GetStorageInfo(defaultPath)
		if err == nil {
			loc.Label = "Default (Home Directory)"
			locations = append(locations, loc)
		}
	}

	// On macOS, scan /Volumes for external and network drives
	if runtime.GOOS == "darwin" {
		entries, err := os.ReadDir("/Volumes")
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != "Macintosh HD" {
					volumePath := filepath.Join("/Volumes", entry.Name())
					loc, err := GetStorageInfo(volumePath)
					if err == nil && loc.IsWritable && loc.IsMounted {
						suggestedPath := filepath.Join(volumePath, "porcupin-ipfs")
						loc.Path = suggestedPath
						// Label is set by GetStorageInfo -> generateLabel using actual volume name
						locations = append(locations, loc)
					}
				}
			}
		}
	}

	// On Linux, check common mount points
	if runtime.GOOS == "linux" {
		mountPoints := []string{"/mnt", "/media"}
		for _, mp := range mountPoints {
			entries, err := os.ReadDir(mp)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					volumePath := filepath.Join(mp, entry.Name())
					loc, err := GetStorageInfo(volumePath)
					if err == nil && loc.IsWritable && loc.IsMounted {
						suggestedPath := filepath.Join(volumePath, "porcupin-ipfs")
						loc.Path = suggestedPath
						locations = append(locations, loc)
					}
				}
			}
		}
	}

	return locations, nil
}
