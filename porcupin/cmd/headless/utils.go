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

	// If repo_path uses ~ (e.g. the default "~/.porcupin/ipfs"), resolve it
	// relative to dataPath when --data was provided. This prevents the path
	// from expanding to /home/<user>/.porcupin/ipfs when running as a
	// different user (e.g. systemd service with --data /var/lib/porcupin).
	if path[0] == '~' {
		// Extract the relative portion after ~/
		relPath := path[1:]
		if len(relPath) > 0 && relPath[0] == '/' {
			relPath = relPath[1:]
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Can't resolve home dir, use dataPath
			return filepath.Join(dataPath, relPath)
		}

		defaultDataDir := filepath.Join(homeDir, ".porcupin")
		if dataPath != defaultDataDir {
			// --data was set to a non-default location, so resolve
			// the repo path relative to dataPath instead of ~
			// e.g. ~/.porcupin/ipfs -> /var/lib/porcupin/ipfs
			trimmed := relPath
			// Strip leading .porcupin/ if present (matches default prefix)
			if len(trimmed) >= len(".porcupin/") && trimmed[:len(".porcupin/")] == ".porcupin/" {
				trimmed = trimmed[len(".porcupin/"):]
			} else if trimmed == ".porcupin" {
				trimmed = ""
			}
			if trimmed == "" {
				return dataPath
			}
			return filepath.Join(dataPath, trimmed)
		}

		// Default --data, expand ~ normally
		return filepath.Join(homeDir, relPath)
	}

	// If absolute, return as is
	if filepath.IsAbs(path) {
		return path
	}

	// If relative, join with dataPath
	return filepath.Join(dataPath, path)
}
