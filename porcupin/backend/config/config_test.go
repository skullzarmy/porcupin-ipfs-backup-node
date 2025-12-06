package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// IPFS Defaults
	if cfg.IPFS.RepoPath != "~/.porcupin/ipfs" {
		t.Errorf("IPFS.RepoPath = %q, want '~/.porcupin/ipfs'", cfg.IPFS.RepoPath)
	}
	if cfg.IPFS.MaxFileSize != 5*1024*1024*1024 {
		t.Errorf("IPFS.MaxFileSize = %d, want 5GB", cfg.IPFS.MaxFileSize)
	}
	if cfg.IPFS.PinTimeout != 2*time.Minute {
		t.Errorf("IPFS.PinTimeout = %v, want 2m", cfg.IPFS.PinTimeout)
	}
	if cfg.IPFS.RateLimit != 10 {
		t.Errorf("IPFS.RateLimit = %d, want 10", cfg.IPFS.RateLimit)
	}

	// Server Defaults
	if cfg.Server.BindAddress != "127.0.0.1:8080" {
		t.Errorf("Server.BindAddress = %q, want '127.0.0.1:8080'", cfg.Server.BindAddress)
	}
	if cfg.Server.EnableAuth {
		t.Error("Server.EnableAuth should be false by default")
	}

	// Backup Defaults
	if cfg.Backup.MaxConcurrency != 5 {
		t.Errorf("Backup.MaxConcurrency = %d, want 5", cfg.Backup.MaxConcurrency)
	}
	if cfg.Backup.MinFreeDiskSpaceGB != 5 {
		t.Errorf("Backup.MinFreeDiskSpaceGB = %d, want 5", cfg.Backup.MinFreeDiskSpaceGB)
	}
	if cfg.Backup.MaxMetadataSizeMB != 1 {
		t.Errorf("Backup.MaxMetadataSizeMB = %d, want 1", cfg.Backup.MaxMetadataSizeMB)
	}
	if cfg.Backup.MaxStorageGB != 0 {
		t.Errorf("Backup.MaxStorageGB = %d, want 0 (unlimited)", cfg.Backup.MaxStorageGB)
	}
	if cfg.Backup.StorageWarningPct != 80 {
		t.Errorf("Backup.StorageWarningPct = %d, want 80", cfg.Backup.StorageWarningPct)
	}
	if !cfg.Backup.SyncOwned {
		t.Error("Backup.SyncOwned should be true by default")
	}
	if !cfg.Backup.SyncCreated {
		t.Error("Backup.SyncCreated should be true by default")
	}

	// TZKT Defaults
	if cfg.TZKT.BaseURL != "https://api.tzkt.io" {
		t.Errorf("TZKT.BaseURL = %q, want 'https://api.tzkt.io'", cfg.TZKT.BaseURL)
	}
}

func TestLoadConfig_NonExistent(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/to/config.yaml")
	if err != nil {
		t.Fatalf("LoadConfig should not error for missing file: %v", err)
	}

	// Should return default values
	defaults := DefaultConfig()
	if cfg.Backup.MaxConcurrency != defaults.Backup.MaxConcurrency {
		t.Errorf("Should return default MaxConcurrency when file doesn't exist")
	}
	if cfg.TZKT.BaseURL != defaults.TZKT.BaseURL {
		t.Errorf("Should return default TZKT.BaseURL when file doesn't exist")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create custom config
	cfg := &Config{
		IPFS: IPFSConfig{
			RepoPath:    "/custom/ipfs/path",
			MaxFileSize: 10 * 1024 * 1024 * 1024, // 10GB
			PinTimeout:  5 * time.Minute,
			RateLimit:   50,
		},
		Server: ServerConfig{
			BindAddress: "0.0.0.0:9090",
			EnableAuth:  true,
			AuthUser:    "admin",
			AuthPass:    "secret123",
		},
		Backup: BackupConfig{
			MaxConcurrency:     10,
			MinFreeDiskSpaceGB: 20,
			MaxMetadataSizeMB:  5,
			MaxStorageGB:       1000,
			StorageWarningPct:  90,
			SyncOwned:          false,
			SyncCreated:        true,
		},
		TZKT: TZKTConfig{
			BaseURL: "https://custom.tzkt.io",
		},
	}

	// Save
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify all values
	// IPFS
	if loaded.IPFS.RepoPath != "/custom/ipfs/path" {
		t.Errorf("IPFS.RepoPath = %q, want '/custom/ipfs/path'", loaded.IPFS.RepoPath)
	}
	if loaded.IPFS.MaxFileSize != 10*1024*1024*1024 {
		t.Errorf("IPFS.MaxFileSize = %d, want 10GB", loaded.IPFS.MaxFileSize)
	}
	if loaded.IPFS.PinTimeout != 5*time.Minute {
		t.Errorf("IPFS.PinTimeout = %v, want 5m", loaded.IPFS.PinTimeout)
	}
	if loaded.IPFS.RateLimit != 50 {
		t.Errorf("IPFS.RateLimit = %d, want 50", loaded.IPFS.RateLimit)
	}

	// Server
	if loaded.Server.BindAddress != "0.0.0.0:9090" {
		t.Errorf("Server.BindAddress = %q, want '0.0.0.0:9090'", loaded.Server.BindAddress)
	}
	if !loaded.Server.EnableAuth {
		t.Error("Server.EnableAuth should be true")
	}
	if loaded.Server.AuthUser != "admin" {
		t.Errorf("Server.AuthUser = %q, want 'admin'", loaded.Server.AuthUser)
	}
	if loaded.Server.AuthPass != "secret123" {
		t.Errorf("Server.AuthPass = %q, want 'secret123'", loaded.Server.AuthPass)
	}

	// Backup
	if loaded.Backup.MaxConcurrency != 10 {
		t.Errorf("Backup.MaxConcurrency = %d, want 10", loaded.Backup.MaxConcurrency)
	}
	if loaded.Backup.MinFreeDiskSpaceGB != 20 {
		t.Errorf("Backup.MinFreeDiskSpaceGB = %d, want 20", loaded.Backup.MinFreeDiskSpaceGB)
	}
	if loaded.Backup.MaxStorageGB != 1000 {
		t.Errorf("Backup.MaxStorageGB = %d, want 1000", loaded.Backup.MaxStorageGB)
	}
	if loaded.Backup.StorageWarningPct != 90 {
		t.Errorf("Backup.StorageWarningPct = %d, want 90", loaded.Backup.StorageWarningPct)
	}
	if loaded.Backup.SyncOwned {
		t.Error("Backup.SyncOwned should be false")
	}
	if !loaded.Backup.SyncCreated {
		t.Error("Backup.SyncCreated should be true")
	}

	// TZKT
	if loaded.TZKT.BaseURL != "https://custom.tzkt.io" {
		t.Errorf("TZKT.BaseURL = %q, want 'https://custom.tzkt.io'", loaded.TZKT.BaseURL)
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use nested path that doesn't exist
	configPath := filepath.Join(tmpDir, "nested", "dir", "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig should create nested directories: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created in nested directory")
	}
}

func TestSaveConfig_AtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write initial config
	cfg1 := DefaultConfig()
	cfg1.Backup.MaxConcurrency = 1
	if err := cfg1.SaveConfig(configPath); err != nil {
		t.Fatalf("First SaveConfig failed: %v", err)
	}

	// Overwrite with new config
	cfg2 := DefaultConfig()
	cfg2.Backup.MaxConcurrency = 99
	if err := cfg2.SaveConfig(configPath); err != nil {
		t.Fatalf("Second SaveConfig failed: %v", err)
	}

	// Verify new values
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Backup.MaxConcurrency != 99 {
		t.Errorf("MaxConcurrency = %d, want 99 (atomic overwrite)", loaded.Backup.MaxConcurrency)
	}

	// Verify no temp files left behind
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if e.Name() != "config.yaml" {
			t.Errorf("Temp file left behind: %s", e.Name())
		}
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("{{invalid yaml::"), 0644); err != nil {
		t.Fatalf("Failed to write invalid YAML: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig should error on invalid YAML")
	}
}

func TestLoadConfig_PartialConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write partial config (only TZKT section)
	partialYAML := `
tzkt:
  base_url: "https://partial.tzkt.io"
`
	if err := os.WriteFile(configPath, []byte(partialYAML), 0644); err != nil {
		t.Fatalf("Failed to write partial YAML: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// TZKT should be overridden
	if loaded.TZKT.BaseURL != "https://partial.tzkt.io" {
		t.Errorf("TZKT.BaseURL = %q, want 'https://partial.tzkt.io'", loaded.TZKT.BaseURL)
	}

	// Other values should be defaults
	defaults := DefaultConfig()
	if loaded.Backup.MaxConcurrency != defaults.Backup.MaxConcurrency {
		t.Errorf("Backup.MaxConcurrency = %d, want default %d", loaded.Backup.MaxConcurrency, defaults.Backup.MaxConcurrency)
	}
}

func TestConfig_DurationParsing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write config with duration
	yamlContent := `
ipfs:
  pin_timeout: 5m30s
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write YAML: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	expected := 5*time.Minute + 30*time.Second
	if loaded.IPFS.PinTimeout != expected {
		t.Errorf("IPFS.PinTimeout = %v, want %v", loaded.IPFS.PinTimeout, expected)
	}
}

func TestIPFSConfig_FileSizeLimits(t *testing.T) {
	cfg := DefaultConfig()

	// Test default max file size
	if cfg.IPFS.MaxFileSize != 5*1024*1024*1024 {
		t.Errorf("Default MaxFileSize = %d bytes, want 5GB", cfg.IPFS.MaxFileSize)
	}

	// Test setting larger limits
	cfg.IPFS.MaxFileSize = 100 * 1024 * 1024 * 1024 // 100GB
	if cfg.IPFS.MaxFileSize != 100*1024*1024*1024 {
		t.Errorf("MaxFileSize = %d bytes, want 100GB", cfg.IPFS.MaxFileSize)
	}
}

func TestBackupConfig_Validation(t *testing.T) {
	cfg := DefaultConfig()

	// MaxConcurrency should be positive
	if cfg.Backup.MaxConcurrency <= 0 {
		t.Errorf("Default MaxConcurrency = %d, should be positive", cfg.Backup.MaxConcurrency)
	}

	// StorageWarningPct should be percentage
	if cfg.Backup.StorageWarningPct < 0 || cfg.Backup.StorageWarningPct > 100 {
		t.Errorf("Default StorageWarningPct = %d, should be 0-100", cfg.Backup.StorageWarningPct)
	}
}

func TestServerConfig_Security(t *testing.T) {
	cfg := DefaultConfig()

	// Default should bind to localhost only
	if cfg.Server.BindAddress != "127.0.0.1:8080" {
		t.Errorf("Default BindAddress = %q, should be localhost", cfg.Server.BindAddress)
	}

	// Auth should be disabled by default
	if cfg.Server.EnableAuth {
		t.Error("Auth should be disabled by default")
	}

	// Auth credentials should be empty by default
	if cfg.Server.AuthUser != "" || cfg.Server.AuthPass != "" {
		t.Error("Auth credentials should be empty by default")
	}
}
