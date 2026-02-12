package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetByDotNotation(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		key      string
		expected string
	}{
		{"backup.max_concurrency", "5"},
		{"backup.min_free_disk_space_gb", "5"},
		{"backup.max_storage_gb", "0"},
		{"backup.storage_warning_pct", "80"},
		{"backup.sync_owned", "true"},
		{"backup.sync_created", "true"},
		{"ipfs.repo_path", "~/.porcupin/ipfs"},
		{"ipfs.swarm_port", "4001"},
		{"ipfs.pin_timeout", "2m0s"},
		{"ipfs.max_file_size", "5368709120"},
		{"ipfs.rate_limit_mbps", "10"},
		{"server.bind_address", "127.0.0.1:8080"},
		{"server.enable_auth", "false"},
		{"tzkt.base_url", "https://api.tzkt.io"},
		{"api.enabled", "false"},
		{"api.port", "8085"},
		{"api.bind", "0.0.0.0"},
		{"api.allow_public", "false"},
		{"api.tls.cert", ""},
		{"api.tls.key", ""},
		{"api.rate_limit.per_ip", "10"},
		{"api.rate_limit.global", "100"},
		{"api.rate_limit.burst", "20"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := GetByDotNotation(cfg, tt.key)
			if err != nil {
				t.Fatalf("GetByDotNotation(%q) error: %v", tt.key, err)
			}
			if got != tt.expected {
				t.Errorf("GetByDotNotation(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

func TestGetByDotNotation_DashNormalization(t *testing.T) {
	cfg := DefaultConfig()

	// Dashes should be treated as underscores
	val, err := GetByDotNotation(cfg, "backup.max-concurrency")
	if err != nil {
		t.Fatalf("GetByDotNotation with dashes error: %v", err)
	}
	if val != "5" {
		t.Errorf("Got %q, want %q", val, "5")
	}

	val, err = GetByDotNotation(cfg, "ipfs.rate-limit-mbps")
	if err != nil {
		t.Fatalf("GetByDotNotation with dashes error: %v", err)
	}
	if val != "10" {
		t.Errorf("Got %q, want %q", val, "10")
	}
}

func TestGetByDotNotation_Errors(t *testing.T) {
	cfg := DefaultConfig()

	// Missing dot
	_, err := GetByDotNotation(cfg, "backup")
	if err == nil {
		t.Error("Expected error for key without dot")
	}

	// Invalid section
	_, err = GetByDotNotation(cfg, "nonexistent.field")
	if err == nil {
		t.Error("Expected error for invalid section")
	}

	// Invalid field
	_, err = GetByDotNotation(cfg, "backup.nonexistent")
	if err == nil {
		t.Error("Expected error for invalid field")
	}
}

func TestSetByDotNotation(t *testing.T) {
	cfg := DefaultConfig()

	// Set integer
	if err := SetByDotNotation(cfg, "backup.max_concurrency", "10"); err != nil {
		t.Fatalf("SetByDotNotation int error: %v", err)
	}
	if cfg.Backup.MaxConcurrency != 10 {
		t.Errorf("MaxConcurrency = %d, want 10", cfg.Backup.MaxConcurrency)
	}

	// Set bool
	if err := SetByDotNotation(cfg, "backup.sync_owned", "false"); err != nil {
		t.Fatalf("SetByDotNotation bool error: %v", err)
	}
	if cfg.Backup.SyncOwned {
		t.Error("SyncOwned should be false")
	}

	// Set string
	if err := SetByDotNotation(cfg, "tzkt.base_url", "https://custom.tzkt.io"); err != nil {
		t.Fatalf("SetByDotNotation string error: %v", err)
	}
	if cfg.TZKT.BaseURL != "https://custom.tzkt.io" {
		t.Errorf("BaseURL = %q, want 'https://custom.tzkt.io'", cfg.TZKT.BaseURL)
	}

	// Set duration
	if err := SetByDotNotation(cfg, "ipfs.pin_timeout", "5m30s"); err != nil {
		t.Fatalf("SetByDotNotation duration error: %v", err)
	}
	expected := 5*time.Minute + 30*time.Second
	if cfg.IPFS.PinTimeout != expected {
		t.Errorf("PinTimeout = %v, want %v", cfg.IPFS.PinTimeout, expected)
	}

	// Set with dashes
	if err := SetByDotNotation(cfg, "backup.max-concurrency", "3"); err != nil {
		t.Fatalf("SetByDotNotation with dashes error: %v", err)
	}
	if cfg.Backup.MaxConcurrency != 3 {
		t.Errorf("MaxConcurrency = %d, want 3", cfg.Backup.MaxConcurrency)
	}

	// Set nested field (api.tls.cert)
	if err := SetByDotNotation(cfg, "api.tls.cert", "/path/to/cert.pem"); err != nil {
		t.Fatalf("SetByDotNotation nested error: %v", err)
	}
	if cfg.API.TLS.Cert != "/path/to/cert.pem" {
		t.Errorf("TLS.Cert = %q, want '/path/to/cert.pem'", cfg.API.TLS.Cert)
	}
}

func TestSetByDotNotation_Errors(t *testing.T) {
	cfg := DefaultConfig()

	// Invalid integer
	err := SetByDotNotation(cfg, "backup.max_concurrency", "not_a_number")
	if err == nil {
		t.Error("Expected error for invalid integer")
	}

	// Invalid boolean
	err = SetByDotNotation(cfg, "backup.sync_owned", "not_a_bool")
	if err == nil {
		t.Error("Expected error for invalid boolean")
	}

	// Invalid duration
	err = SetByDotNotation(cfg, "ipfs.pin_timeout", "not_a_duration")
	if err == nil {
		t.Error("Expected error for invalid duration")
	}

	// Invalid key
	err = SetByDotNotation(cfg, "backup.nonexistent", "value")
	if err == nil {
		t.Error("Expected error for invalid key")
	}
}

func TestListAll(t *testing.T) {
	cfg := DefaultConfig()
	items := ListAll(cfg)

	if len(items) == 0 {
		t.Fatal("ListAll returned no items")
	}

	// Check that all items have non-empty keys
	for _, item := range items {
		if item.Key == "" {
			t.Error("Found item with empty key")
		}
	}

	// Check that expected keys are present
	expectedKeys := map[string]bool{
		"backup.max_concurrency":      false,
		"ipfs.repo_path":              false,
		"server.bind_address":         false,
		"tzkt.base_url":               false,
		"api.port":                    false,
		"api.tls.cert":                false,
		"api.rate_limit.per_ip":       false,
	}

	for _, item := range items {
		if _, ok := expectedKeys[item.Key]; ok {
			expectedKeys[item.Key] = true
		}
	}

	for key, found := range expectedKeys {
		if !found {
			t.Errorf("Expected key %q not found in ListAll", key)
		}
	}
}

func TestValidKeys(t *testing.T) {
	keys := ValidKeys()
	if len(keys) == 0 {
		t.Fatal("ValidKeys returned no keys")
	}

	// Should be sorted
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			t.Errorf("Keys not sorted: %q comes after %q", keys[i], keys[i-1])
		}
	}
}

func TestNormalizeDotKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"backup.max-concurrency", "backup.max_concurrency"},
		{"backup.max_concurrency", "backup.max_concurrency"},
		{"ipfs.rate-limit-mbps", "ipfs.rate_limit_mbps"},
		{"api.tls.cert", "api.tls.cert"},
		{"api.rate-limit.per-ip", "api.rate_limit.per_ip"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeDotKey(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeDotKey(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEnsureConfigFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// First call should create the file
	created, err := EnsureConfigFile(configPath)
	if err != nil {
		t.Fatalf("EnsureConfigFile error: %v", err)
	}
	if !created {
		t.Error("Expected created=true for new file")
	}

	// Verify file exists and is valid
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig after EnsureConfigFile error: %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Backup.MaxConcurrency != defaults.Backup.MaxConcurrency {
		t.Errorf("MaxConcurrency = %d, want %d", cfg.Backup.MaxConcurrency, defaults.Backup.MaxConcurrency)
	}

	// Second call should not create
	created, err = EnsureConfigFile(configPath)
	if err != nil {
		t.Fatalf("Second EnsureConfigFile error: %v", err)
	}
	if created {
		t.Error("Expected created=false for existing file")
	}
}

func TestEnsureConfigFile_NestedDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "nested", "dir", "config.yaml")

	created, err := EnsureConfigFile(configPath)
	if err != nil {
		t.Fatalf("EnsureConfigFile nested error: %v", err)
	}
	if !created {
		t.Error("Expected created=true")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created in nested directory")
	}
}
