package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	IPFS   IPFSConfig   `yaml:"ipfs"`
	Server ServerConfig `yaml:"server"`
	Backup BackupConfig `yaml:"backup"`
	TZKT   TZKTConfig   `yaml:"tzkt"`
	API    APIConfig    `yaml:"api"`
}

// IPFSConfig holds IPFS-specific configuration
type IPFSConfig struct {
	RepoPath    string        `yaml:"repo_path" json:"repo_path"`
	SwarmPort   int           `yaml:"swarm_port" json:"swarm_port"`           // IPFS swarm port for p2p connections (default 4001)
	MaxFileSize int64         `yaml:"max_file_size" json:"max_file_size"`     // in bytes
	PinTimeout  time.Duration `yaml:"pin_timeout" json:"pin_timeout"`         // timeout for pin operations
	RateLimit   int           `yaml:"rate_limit_mbps" json:"rate_limit_mbps"` // bandwidth limit in Mbps

	// DelegatedRouters lists the HTTP /routing/v1 endpoints used to discover
	// which peers host a given CID, queried in parallel with the DHT.
	//
	// The special value "auto" expands (via Kubo AutoConf) to the managed IPNI
	// indexer (cid.contact). This is required for most NFT content — Versum,
	// Emprops, and anything stored via nft.storage/web3.storage/Filecoin —
	// which advertises providers to IPNI but NOT the Amino DHT.
	//
	// Default: ["auto"]. Add entries to query additional routers, e.g.
	// ["auto", "https://my-router.example/routing/v1"]. An explicit empty list
	// disables delegated routing (DHT only). Invalid entries are ignored.
	DelegatedRouters []string `yaml:"delegated_routers" json:"delegated_routers"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	BindAddress string `yaml:"bind_address"`
	EnableAuth  bool   `yaml:"enable_auth"`
	AuthUser    string `yaml:"auth_user"`
	AuthPass    string `yaml:"auth_pass"`
}

// BackupConfig holds backup-specific configuration
type BackupConfig struct {
	MaxConcurrency     int  `yaml:"max_concurrency" json:"max_concurrency"`               // max concurrent workers
	MinFreeDiskSpaceGB int  `yaml:"min_free_disk_space_gb" json:"min_free_disk_space_gb"` // minimum free disk space in GB
	MaxMetadataSizeMB  int  `yaml:"max_metadata_size_mb" json:"max_metadata_size_mb"`     // max metadata size in MB
	MaxStorageGB       int  `yaml:"max_storage_gb" json:"max_storage_gb"`                 // max storage allocation in GB (0 = unlimited)
	StorageWarningPct  int  `yaml:"storage_warning_pct" json:"storage_warning_pct"`       // warn when storage reaches this % (default 80)
	SyncOwned          bool `yaml:"sync_owned" json:"sync_owned"`                         // default: sync owned NFTs for new wallets
	SyncCreated        bool `yaml:"sync_created" json:"sync_created"`                     // default: sync created NFTs for new wallets
}

// TZKTConfig holds TZKT API configuration
type TZKTConfig struct {
	BaseURL string `yaml:"base_url"`
}

// APIConfig holds REST API server configuration
type APIConfig struct {
	Enabled     bool            `yaml:"enabled" json:"enabled"`           // Set to true by --serve
	Port        int             `yaml:"port" json:"port"`                 // Default 8085
	Bind        string          `yaml:"bind" json:"bind"`                 // Default "0.0.0.0"
	AllowPublic bool            `yaml:"allow_public" json:"allow_public"` // Override IP restrictions
	TLS         APITLSConfig    `yaml:"tls" json:"tls"`
	RateLimit   RateLimitConfig `yaml:"rate_limit" json:"rate_limit"`
}

// APITLSConfig holds TLS configuration for the API server
type APITLSConfig struct {
	Cert string `yaml:"cert" json:"cert"` // Path to TLS certificate
	Key  string `yaml:"key" json:"key"`   // Path to TLS private key
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	PerIP  int `yaml:"per_ip" json:"per_ip"` // Requests per second per IP
	Global int `yaml:"global" json:"global"` // Total requests per second
	Burst  int `yaml:"burst" json:"burst"`   // Burst allowance
}

// DefaultConfig returns a configuration with secure defaults
func DefaultConfig() *Config {
	return &Config{
		IPFS: IPFSConfig{
			RepoPath:    "~/.porcupin/ipfs",
			SwarmPort:   4001,                   // default IPFS swarm port
			MaxFileSize: 5 * 1024 * 1024 * 1024, // 5GB
			PinTimeout:  2 * time.Minute,
			RateLimit:   10, // 10 Mbps
			// "auto" expands to the IPNI indexer (cid.contact) via Kubo AutoConf.
			DelegatedRouters: []string{"auto"},
		},
		Server: ServerConfig{
			BindAddress: "127.0.0.1:8080", // localhost only by default
			EnableAuth:  false,            // opt-in auth
			AuthUser:    "",
			AuthPass:    "",
		},
		Backup: BackupConfig{
			MaxConcurrency:     5,
			MinFreeDiskSpaceGB: 5,
			MaxMetadataSizeMB:  1,
			MaxStorageGB:       0,    // unlimited by default
			StorageWarningPct:  80,   // warn at 80%
			SyncOwned:          true, // sync owned by default
			SyncCreated:        true, // sync created by default
		},
		TZKT: TZKTConfig{
			BaseURL: "https://api.tzkt.io",
		},
		API: APIConfig{
			Enabled:     false,
			Port:        8085,
			Bind:        "0.0.0.0",
			AllowPublic: false,
			TLS: APITLSConfig{
				Cert: "",
				Key:  "",
			},
			RateLimit: RateLimitConfig{
				PerIP:  10,  // 10 requests per second per IP
				Global: 100, // 100 requests per second total
				Burst:  20,  // Burst allowance
			},
		},
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves configuration to a YAML file
// Uses atomic write pattern: write to temp file, sync, then rename
func (c *Config) SaveConfig(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	// Create temp file in same directory (ensures same filesystem for rename)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Write data to temp file
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Sync to ensure data is on disk before rename
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync config: %w", err)
	}

	// Close before rename (required on Windows)
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tmpFile = nil // Prevent defer from double-closing

	// Atomic rename (on same filesystem, this is atomic on all platforms)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// EnsureConfigFile creates the config file with defaults if it doesn't exist.
// Returns true if a new file was created, false if it already existed.
func EnsureConfigFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		// File already exists
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	// File doesn't exist — write defaults
	cfg := DefaultConfig()
	if err := cfg.SaveConfig(path); err != nil {
		return false, fmt.Errorf("failed to create default config: %w", err)
	}
	return true, nil
}
