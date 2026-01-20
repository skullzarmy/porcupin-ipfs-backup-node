package main

import (
	"os"
	"path/filepath"

	"porcupin/backend/config"
)

// resolveRepoPath determines the final IPFS repo path based on config and data dir
func resolveRepoPath(cfg *config.Config, dataPath string) string {
	path := cfg.IPFS.RepoPath
	if path == "" {
		return filepath.Join(dataPath, "ipfs")
	}
	
	// Handle home directory expansion
	if path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(homeDir, path[1:])
		}
	}
	
	// If absolute, return as is
	if filepath.IsAbs(path) {
		return path
	}
	
	// If relative, join with dataPath
	return filepath.Join(dataPath, path)
}
