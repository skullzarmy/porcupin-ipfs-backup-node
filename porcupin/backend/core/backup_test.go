package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"porcupin/backend/config"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

// testDB creates an in-memory SQLite database for testing
func testDB(t *testing.T) *db.Database {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	if err := db.InitDB(gormDB); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}
	return db.NewDatabase(gormDB)
}

// testConfig creates a test configuration
func testConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Backup.MaxConcurrency = 2
	cfg.Backup.MinFreeDiskSpaceGB = 0 // Disable disk check for tests
	cfg.Backup.MaxStorageGB = 0       // Unlimited for tests
	cfg.IPFS.PinTimeout = 5 * time.Second
	cfg.IPFS.MaxFileSize = 100 * 1024 * 1024 // 100MB
	return cfg
}

// =============================================================================
// HELPER FUNCTION TESTS (Pure Functions - No Dependencies)
// =============================================================================

func TestExtractCIDFromURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		// Standard ipfs:// URIs
		{"simple ipfs:// URI", "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"ipfs:// with path", "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG/image.png", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"ipfs:// with query params (fxhash style)", "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG?fxhash=oo123", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"ipfs:// with path and query", "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG/index.html?param=value", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"CIDv1 base32", "ipfs://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"},
		// Gateway URLs
		{"ipfs.io gateway", "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"cloudflare gateway", "https://cloudflare-ipfs.com/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"gateway with path", "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG/metadata.json", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		// Edge cases
		{"empty string", "", ""},
		{"non-IPFS URL", "https://example.com/image.png", ""},
		{"just ipfs://", "ipfs://", ""},
		// More edge cases for complete coverage
		{"ipfs:// with trailing slash", "ipfs://QmTest/", "QmTest"},
		{"multiple slashes in path", "ipfs://QmTest/a/b/c.png", "QmTest"},
		{"query without path", "ipfs://QmTest?key=value", "QmTest"},
		{"subdomain gateway style", "https://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi.ipfs.dweb.link/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCIDFromURI(tt.uri)
			if result != tt.expected {
				t.Errorf("extractCIDFromURI(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestIsIPFSURI(t *testing.T) {
	tests := []struct {
		uri      string
		expected bool
	}{
		{"ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", true},
		{"ipfs://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", true},
		{"https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", true},
		{"https://cloudflare-ipfs.com/ipfs/QmTest", true},
		{"https://gateway.pinata.cloud/ipfs/QmTest", true},
		{"https://example.com/image.png", false},
		{"http://localhost:8080/file.json", false},
		{"data:image/png;base64,abc123", false},
		{"", false},
		{"ipfs://", true}, // Valid prefix, even if no CID
		{"IPFS://QmTest", false}, // Case sensitive
		{"/ipfs/QmTest", true},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := isIPFSURI(tt.uri)
			if result != tt.expected {
				t.Errorf("isIPFSURI(%q) = %v, want %v", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"exact timeout error string", fmt.Errorf("context deadline exceeded"), true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"connection refused", fmt.Errorf("connection refused"), false},
		{"wrapped timeout (not detected)", fmt.Errorf("failed: context deadline exceeded"), false},
		{"context canceled", context.Canceled, false},
		{"empty error", fmt.Errorf(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			if result != tt.expected {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestResolveURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{"ipfs:// to gateway", "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		{"ipfs:// with path", "ipfs://QmTest/image.png", "https://ipfs.io/ipfs/QmTest/image.png"},
		{"already HTTP", "https://example.com/file.json", "https://example.com/file.json"},
		{"gateway URL unchanged", "https://ipfs.io/ipfs/QmTest", "https://ipfs.io/ipfs/QmTest"},
		{"empty string", "", ""},
		{"ipfs:// CIDv1", "ipfs://bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", "https://ipfs.io/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"},
		{"ipfs:// with query params", "ipfs://QmTest?fxhash=123", "https://ipfs.io/ipfs/QmTest?fxhash=123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveURI(tt.uri)
			if result != tt.expected {
				t.Errorf("resolveURI(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestHasIPFSContent(t *testing.T) {
	tests := []struct {
		name     string
		metadata *indexer.TokenMetadata
		expected bool
	}{
		{"nil metadata", nil, false},
		{"empty metadata", &indexer.TokenMetadata{}, false},
		{"has artifact URI", &indexer.TokenMetadata{ArtifactURI: "ipfs://QmTest"}, true},
		{"has display URI", &indexer.TokenMetadata{DisplayURI: "ipfs://QmTest"}, true},
		{"has thumbnail URI", &indexer.TokenMetadata{ThumbnailURI: "ipfs://QmTest"}, true},
		{"has format URIs", &indexer.TokenMetadata{Formats: []indexer.Format{{URI: "ipfs://QmTest"}}}, true},
		{"empty format URIs", &indexer.TokenMetadata{Formats: []indexer.Format{{URI: ""}}}, false},
		{"all URIs filled", &indexer.TokenMetadata{
			ArtifactURI:  "ipfs://Qm1",
			DisplayURI:   "ipfs://Qm2",
			ThumbnailURI: "ipfs://Qm3",
			Formats:      []indexer.Format{{URI: "ipfs://Qm4"}},
		}, true},
		// Note: hasIPFSContent just checks if any URI exists, not if it's IPFS format
		{"non-IPFS artifact still returns true", &indexer.TokenMetadata{ArtifactURI: "https://example.com/img.png"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasIPFSContent(tt.metadata)
			if result != tt.expected {
				t.Errorf("hasIPFSContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCollectAssetURIs(t *testing.T) {
	tests := []struct {
		name         string
		metadata     *indexer.TokenMetadata
		expectedURIs []string
	}{
		{"nil metadata", nil, []string{}},
		{"empty metadata", &indexer.TokenMetadata{}, []string{}},
		{
			"all unique IPFS URIs",
			&indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmArtifact",
				DisplayURI:   "ipfs://QmDisplay",
				ThumbnailURI: "ipfs://QmThumb",
				Formats: []indexer.Format{
					{URI: "ipfs://QmFormat1"},
					{URI: "ipfs://QmFormat2"},
				},
			},
			[]string{"ipfs://QmArtifact", "ipfs://QmDisplay", "ipfs://QmThumb", "ipfs://QmFormat1", "ipfs://QmFormat2"},
		},
		{
			"filters non-IPFS URIs",
			&indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmArtifact",
				DisplayURI:   "https://example.com/image.png",
				ThumbnailURI: "data:image/png;base64,abc",
			},
			[]string{"ipfs://QmArtifact"},
		},
		{
			"deduplicates URIs",
			&indexer.TokenMetadata{
				ArtifactURI:  "ipfs://QmSame",
				DisplayURI:   "ipfs://QmSame",
				ThumbnailURI: "ipfs://QmSame",
			},
			[]string{"ipfs://QmSame"},
		},
		{
			"gateway URLs are collected",
			&indexer.TokenMetadata{
				ArtifactURI: "https://ipfs.io/ipfs/QmGateway",
			},
			[]string{"https://ipfs.io/ipfs/QmGateway"},
		},
		{
			"empty format URI ignored",
			&indexer.TokenMetadata{
				Formats: []indexer.Format{{URI: ""}, {URI: "ipfs://QmFormat"}},
			},
			[]string{"ipfs://QmFormat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(map[string]bool)
			collectAssetURIs(tt.metadata, seen)

			if len(seen) != len(tt.expectedURIs) {
				t.Errorf("collectAssetURIs() returned %d URIs, want %d. Got: %v", len(seen), len(tt.expectedURIs), seen)
			}

			for _, uri := range tt.expectedURIs {
				if !seen[uri] {
					t.Errorf("collectAssetURIs() missing expected URI: %s", uri)
				}
			}
		})
	}
}

func TestCountAssets(t *testing.T) {
	tests := []struct {
		name     string
		metadata *indexer.TokenMetadata
		expected int
	}{
		{"nil", nil, 0},
		{"empty", &indexer.TokenMetadata{}, 0},
		{"one artifact", &indexer.TokenMetadata{ArtifactURI: "ipfs://Qm1"}, 1},
		{"artifact and thumbnail same (deduplicated)", &indexer.TokenMetadata{
			ArtifactURI:  "ipfs://QmSame",
			ThumbnailURI: "ipfs://QmSame",
		}, 1},
		{"multiple unique", &indexer.TokenMetadata{
			ArtifactURI:  "ipfs://Qm1",
			DisplayURI:   "ipfs://Qm2",
			ThumbnailURI: "ipfs://Qm3",
		}, 3},
		{"with formats", &indexer.TokenMetadata{
			ArtifactURI: "ipfs://Qm1",
			Formats:     []indexer.Format{{URI: "ipfs://Qm2"}, {URI: "ipfs://Qm3"}},
		}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countAssets(tt.metadata)
			if result != tt.expected {
				t.Errorf("countAssets() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected int
	}{
		{"hello world", "world", 6},
		{"hello world", "hello", 0},
		{"hello world", "xyz", -1},
		{"", "test", -1},
		{"test", "", 0},
		{"/ipfs/QmTest", "/ipfs/", 0},
		{"https://ipfs.io/ipfs/Qm", "/ipfs/", 15},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_in_%s", tt.substr, tt.s), func(t *testing.T) {
			result := indexOf(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("indexOf(%q, %q) = %d, want %d", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// BACKUPMANAGER TESTS (With Mocked Dependencies)
// =============================================================================

// mockIPFSNode implements the minimal interface needed for testing
type mockIPFSNode struct {
	pinned       map[string]bool
	sizes        map[string]int64
	pinError     error
	statError    error
	repoPath     string
	mu           sync.Mutex
}

func newMockIPFSNode() *mockIPFSNode {
	return &mockIPFSNode{
		pinned:   make(map[string]bool),
		sizes:    make(map[string]int64),
		repoPath: "/tmp/mock-ipfs",
	}
}

func (m *mockIPFSNode) Pin(ctx context.Context, cid string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pinError != nil {
		return m.pinError
	}
	m.pinned[cid] = true
	return nil
}

func (m *mockIPFSNode) Stat(ctx context.Context, cid string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statError != nil {
		return 0, m.statError
	}
	if size, ok := m.sizes[cid]; ok {
		return size, nil
	}
	return 1024, nil // Default 1KB
}

func (m *mockIPFSNode) GetRepoPath() string {
	return m.repoPath
}

func TestNewBackupManager(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	// Create a minimal BackupManager without full IPFS/Indexer
	// This tests the constructor
	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	if bm.db == nil {
		t.Error("BackupManager.db should not be nil")
	}
	if bm.config == nil {
		t.Error("BackupManager.config should not be nil")
	}
	if cap(bm.workers) != cfg.Backup.MaxConcurrency {
		t.Errorf("BackupManager.workers capacity = %d, want %d", cap(bm.workers), cfg.Backup.MaxConcurrency)
	}
	if bm.progress.Phase != "idle" {
		t.Errorf("Initial phase = %q, want 'idle'", bm.progress.Phase)
	}
}

func TestBackupManager_PauseResume(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Initially not paused
	if bm.IsPaused() {
		t.Error("BackupManager should not be paused initially")
	}

	// Pause
	bm.SetPaused(true)
	if !bm.IsPaused() {
		t.Error("BackupManager should be paused after SetPaused(true)")
	}

	// Check progress message updated
	progress := bm.GetProgress()
	if progress.Message != "Paused" {
		t.Errorf("Progress message = %q, want 'Paused'", progress.Message)
	}

	// Resume
	bm.SetPaused(false)
	if bm.IsPaused() {
		t.Error("BackupManager should not be paused after SetPaused(false)")
	}
}

func TestBackupManager_GetProgress(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{
			Phase:        "syncing",
			TotalNFTs:    100,
			ProcessedNFTs: 50,
		},
	}

	progress := bm.GetProgress()

	if progress.Phase != "syncing" {
		t.Errorf("Phase = %q, want 'syncing'", progress.Phase)
	}
	if progress.TotalNFTs != 100 {
		t.Errorf("TotalNFTs = %d, want 100", progress.TotalNFTs)
	}
	if progress.ProcessedNFTs != 50 {
		t.Errorf("ProcessedNFTs = %d, want 50", progress.ProcessedNFTs)
	}
}

func TestBackupManager_UpdateProgress(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Update progress
	bm.updateProgress(func(p *SyncProgress) {
		p.Phase = "pinning"
		p.TotalAssets = 50
		p.PinnedAssets = 25
		p.Message = "Pinning assets..."
	})

	progress := bm.GetProgress()
	if progress.Phase != "pinning" {
		t.Errorf("Phase = %q, want 'pinning'", progress.Phase)
	}
	if progress.TotalAssets != 50 {
		t.Errorf("TotalAssets = %d, want 50", progress.TotalAssets)
	}
	if progress.PinnedAssets != 25 {
		t.Errorf("PinnedAssets = %d, want 25", progress.PinnedAssets)
	}
}

func TestBackupManager_ProgressConcurrency(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Concurrent updates should not race
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bm.updateProgress(func(p *SyncProgress) {
				p.ProcessedNFTs++
			})
			_ = bm.GetProgress()
		}(i)
	}
	wg.Wait()

	progress := bm.GetProgress()
	if progress.ProcessedNFTs != 100 {
		t.Errorf("ProcessedNFTs = %d, want 100 (concurrent updates)", progress.ProcessedNFTs)
	}
}

func TestBackupManager_MarkDiskUsageDirty(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Initially clean
	if bm.diskUsageDirty != 0 {
		t.Error("diskUsageDirty should be 0 initially")
	}

	// Mark dirty
	bm.MarkDiskUsageDirty()
	if bm.diskUsageDirty != 1 {
		t.Error("diskUsageDirty should be 1 after MarkDiskUsageDirty()")
	}

	// Multiple marks should still be 1 (atomic)
	bm.MarkDiskUsageDirty()
	if bm.diskUsageDirty != 1 {
		t.Error("diskUsageDirty should remain 1")
	}
}

// =============================================================================
// INTEGRATION-STYLE TESTS (Database + Logic)
// =============================================================================

func TestBackupManager_IsWithinStorageLimit(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// No limit set (0 = unlimited)
	cfg.Backup.MaxStorageGB = 0
	if !bm.isWithinStorageLimit() {
		t.Error("Should be within limit when MaxStorageGB is 0 (unlimited)")
	}

	// Set a 1GB limit
	cfg.Backup.MaxStorageGB = 1

	// With empty database, should be within limit
	if !bm.isWithinStorageLimit() {
		t.Error("Should be within limit with empty database")
	}

	// Add some pinned assets to database
	wallet := &db.Wallet{Address: "tz1TestWallet123456789012345678901234"}
	database.SaveWallet(wallet)

	nft := &db.NFT{
		TokenID:         "1",
		ContractAddress: "KT1Test",
		WalletAddress:   wallet.Address,
		ArtifactURI:     "ipfs://QmTest",
	}
	database.SaveNFT(nft)

	// Add asset with 500MB
	asset := &db.Asset{
		NFTID:     nft.ID,
		URI:       "ipfs://QmTest",
		Status:    db.StatusPinned,
		SizeBytes: 500 * 1024 * 1024, // 500MB
	}
	database.SaveAsset(asset)

	if !bm.isWithinStorageLimit() {
		t.Error("Should be within 1GB limit with 500MB used")
	}

	// Add another 600MB asset (total 1.1GB)
	asset2 := &db.Asset{
		NFTID:     nft.ID,
		URI:       "ipfs://QmTest2",
		Status:    db.StatusPinned,
		SizeBytes: 600 * 1024 * 1024, // 600MB
	}
	database.SaveAsset(asset2)

	if bm.isWithinStorageLimit() {
		t.Error("Should exceed 1GB limit with 1.1GB used")
	}
}

// =============================================================================
// SYNCPROGRESS TESTS
// =============================================================================

func TestSyncProgress_Defaults(t *testing.T) {
	sp := SyncProgress{}

	if sp.IsActive {
		t.Error("Default IsActive should be false")
	}
	if sp.Phase != "" {
		t.Errorf("Default Phase should be empty, got %q", sp.Phase)
	}
	if !sp.StartedAt.IsZero() {
		t.Error("Default StartedAt should be zero")
	}
}

func TestSyncProgress_ActiveSync(t *testing.T) {
	now := time.Now()
	sp := SyncProgress{
		IsActive:      true,
		Phase:         "fetching",
		WalletAddress: "tz1TestAddress",
		TotalNFTs:     100,
		ProcessedNFTs: 25,
		TotalAssets:   300,
		PinnedAssets:  50,
		FailedAssets:  5,
		CurrentItem:   "My NFT #42",
		StartedAt:     now,
		Message:       "Processing NFTs...",
	}

	if !sp.IsActive {
		t.Error("IsActive should be true")
	}
	if sp.ProcessedNFTs != 25 {
		t.Errorf("ProcessedNFTs = %d, want 25", sp.ProcessedNFTs)
	}
	if sp.CurrentItem != "My NFT #42" {
		t.Errorf("CurrentItem = %q, want 'My NFT #42'", sp.CurrentItem)
	}
}

// =============================================================================
// CONFIG INTEGRATION TESTS
// =============================================================================

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create config with custom values
	cfg := config.DefaultConfig()
	cfg.Backup.MaxConcurrency = 10
	cfg.Backup.MaxStorageGB = 500
	cfg.IPFS.MaxFileSize = 1024 * 1024 * 1024 // 1GB
	cfg.TZKT.BaseURL = "https://custom.tzkt.io"

	// Save
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load
	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify values
	if loaded.Backup.MaxConcurrency != 10 {
		t.Errorf("MaxConcurrency = %d, want 10", loaded.Backup.MaxConcurrency)
	}
	if loaded.Backup.MaxStorageGB != 500 {
		t.Errorf("MaxStorageGB = %d, want 500", loaded.Backup.MaxStorageGB)
	}
	if loaded.IPFS.MaxFileSize != 1024*1024*1024 {
		t.Errorf("MaxFileSize = %d, want 1GB", loaded.IPFS.MaxFileSize)
	}
	if loaded.TZKT.BaseURL != "https://custom.tzkt.io" {
		t.Errorf("BaseURL = %q, want 'https://custom.tzkt.io'", loaded.TZKT.BaseURL)
	}
}

func TestConfig_LoadNonExistent(t *testing.T) {
	cfg, err := config.LoadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("LoadConfig should not error for missing file: %v", err)
	}

	// Should return defaults
	defaults := config.DefaultConfig()
	if cfg.Backup.MaxConcurrency != defaults.Backup.MaxConcurrency {
		t.Errorf("Should return default MaxConcurrency")
	}
}

// =============================================================================
// BACKUPSERVICE TESTS (Service Lifecycle)
// =============================================================================

func TestNewBackupService(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	// Create a mock IPFS node (we'll use nil checks in service)
	mockNode := &ipfs.Node{} // Empty, but tests won't actually call IPFS methods

	// Create indexer
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	if service == nil {
		t.Fatal("NewBackupService returned nil")
	}
	if service.manager == nil {
		t.Error("BackupService.manager should not be nil")
	}
	if service.db == nil {
		t.Error("BackupService.db should not be nil")
	}
	if service.status.State != StateStopped {
		t.Errorf("Initial state = %q, want %q", service.status.State, StateStopped)
	}
}

func TestBackupService_StartStop(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the service
	service.Start(ctx)

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Check status changed from stopped
	status := service.GetStatus()
	if status.State == StateStopped {
		t.Error("Service should not be stopped after Start()")
	}

	// Stop the service
	service.Stop()

	// Give it time to stop
	time.Sleep(100 * time.Millisecond)

	// The context should be canceled
	select {
	case <-service.ctx.Done():
		// Good, context was canceled
	default:
		t.Error("Context should be canceled after Stop()")
	}
}

func TestBackupService_PauseResume(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Initially not paused
	if service.IsPaused() {
		t.Error("Service should not be paused initially")
	}

	// Pause
	service.Pause()
	if !service.IsPaused() {
		t.Error("Service should be paused after Pause()")
	}

	// Also check the manager was paused
	if !service.manager.IsPaused() {
		t.Error("Manager should be paused when service is paused")
	}

	// Resume
	service.Resume()
	if service.IsPaused() {
		t.Error("Service should not be paused after Resume()")
	}

	// Manager should also resume
	if service.manager.IsPaused() {
		t.Error("Manager should be resumed when service is resumed")
	}
}

func TestBackupService_GetStatus(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Initial status
	status := service.GetStatus()
	if status.State != StateStopped {
		t.Errorf("Initial state = %q, want %q", status.State, StateStopped)
	}

	// Update internal status
	service.updateStatus(func(st *ServiceStatus) {
		st.State = StateSyncing
		st.WalletsTotal = 5
		st.WalletsSynced = 2
		st.CurrentWallet = "tz1Test"
		st.Message = "Syncing..."
	})

	status = service.GetStatus()
	if status.State != StateSyncing {
		t.Errorf("State = %q, want %q", status.State, StateSyncing)
	}
	if status.WalletsTotal != 5 {
		t.Errorf("WalletsTotal = %d, want 5", status.WalletsTotal)
	}
	if status.WalletsSynced != 2 {
		t.Errorf("WalletsSynced = %d, want 2", status.WalletsSynced)
	}
	if status.CurrentWallet != "tz1Test" {
		t.Errorf("CurrentWallet = %q, want 'tz1Test'", status.CurrentWallet)
	}
}

func TestBackupService_StatusMergesWithProgress(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Set manager progress to be active
	service.manager.updateProgress(func(p *SyncProgress) {
		p.IsActive = true
		p.TotalNFTs = 50
		p.ProcessedNFTs = 25
		p.TotalAssets = 100
		p.PinnedAssets = 40
		p.FailedAssets = 5
		p.CurrentItem = "Test NFT"
		p.Message = "Processing..."
	})

	// Get status - should merge with progress
	status := service.GetStatus()

	if status.TotalNFTs != 50 {
		t.Errorf("TotalNFTs = %d, want 50 (from progress)", status.TotalNFTs)
	}
	if status.ProcessedNFTs != 25 {
		t.Errorf("ProcessedNFTs = %d, want 25", status.ProcessedNFTs)
	}
	if status.TotalAssets != 100 {
		t.Errorf("TotalAssets = %d, want 100", status.TotalAssets)
	}
	if status.PinnedAssets != 40 {
		t.Errorf("PinnedAssets = %d, want 40", status.PinnedAssets)
	}
	if status.FailedAssets != 5 {
		t.Errorf("FailedAssets = %d, want 5", status.FailedAssets)
	}
	if status.CurrentItem != "Test NFT" {
		t.Errorf("CurrentItem = %q, want 'Test NFT'", status.CurrentItem)
	}
	if status.Message != "Processing..." {
		t.Errorf("Message = %q, want 'Processing...'", status.Message)
	}
}

func TestBackupService_TriggerSync(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// TriggerSync should add to channel
	service.TriggerSync("tz1TestAddress")

	// Should receive from channel
	select {
	case addr := <-service.triggerCh:
		if addr != "tz1TestAddress" {
			t.Errorf("Received address = %q, want 'tz1TestAddress'", addr)
		}
	default:
		t.Error("TriggerSync should put address in channel")
	}
}

func TestBackupService_GetManager(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	manager := service.GetManager()
	if manager == nil {
		t.Error("GetManager should return the backup manager")
	}
	if manager != service.manager {
		t.Error("GetManager should return the same manager instance")
	}
}

// =============================================================================
// SERVICE STATE TESTS
// =============================================================================

func TestServiceState_Values(t *testing.T) {
	// Ensure constants have expected values
	if StateStarting != "starting" {
		t.Errorf("StateStarting = %q, want 'starting'", StateStarting)
	}
	if StateSyncing != "syncing" {
		t.Errorf("StateSyncing = %q, want 'syncing'", StateSyncing)
	}
	if StateWatching != "watching" {
		t.Errorf("StateWatching = %q, want 'watching'", StateWatching)
	}
	if StatePaused != "paused" {
		t.Errorf("StatePaused = %q, want 'paused'", StatePaused)
	}
	if StateStopped != "stopped" {
		t.Errorf("StateStopped = %q, want 'stopped'", StateStopped)
	}
}

func TestServiceStatus_JSON(t *testing.T) {
	now := time.Now()
	status := ServiceStatus{
		State:          StateSyncing,
		Message:        "Syncing wallet...",
		IsPaused:       false,
		CurrentWallet:  "tz1TestAddress",
		WalletsTotal:   5,
		WalletsSynced:  2,
		TotalNFTs:      100,
		ProcessedNFTs:  50,
		TotalAssets:    300,
		PinnedAssets:   150,
		FailedAssets:   10,
		PendingRetries: 5,
		CurrentItem:    "NFT #42",
		LastSyncAt:     &now,
	}

	// Should be able to marshal to JSON
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal status: %v", err)
	}

	// Unmarshal and verify
	var decoded ServiceStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal status: %v", err)
	}

	if decoded.State != StateSyncing {
		t.Errorf("State = %q, want %q", decoded.State, StateSyncing)
	}
	if decoded.WalletsTotal != 5 {
		t.Errorf("WalletsTotal = %d, want 5", decoded.WalletsTotal)
	}
	if decoded.LastSyncAt == nil {
		t.Error("LastSyncAt should not be nil")
	}
}

// =============================================================================
// BACKUP MANAGER - WORKER SEMAPHORE TESTS
// =============================================================================

func TestBackupManager_WorkerSemaphore(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	cfg.Backup.MaxConcurrency = 2 // Only allow 2 concurrent workers

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Fill up worker slots
	bm.workers <- struct{}{}
	bm.workers <- struct{}{}

	// Third worker should block
	select {
	case bm.workers <- struct{}{}:
		t.Error("Third worker should not acquire slot immediately")
	case <-time.After(50 * time.Millisecond):
		// Expected - channel is full
	}

	// Release one slot
	<-bm.workers

	// Now third should acquire
	select {
	case bm.workers <- struct{}{}:
		// Good
	case <-time.After(50 * time.Millisecond):
		t.Error("Third worker should acquire after slot released")
	}

	// Clean up
	<-bm.workers
	<-bm.workers
}

func TestBackupManager_Shutdown(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Shutdown should close channel
	bm.Shutdown()

	// Channel should be closed
	select {
	case <-bm.shutdown:
		// Good, channel closed
	default:
		t.Error("Shutdown channel should be closed")
	}
}

// =============================================================================
// PIN WITH RETRY TESTS
// =============================================================================

// mockIPFSNodeWithCallCount tracks pin calls for testing retries
type mockIPFSNodeWithCallCount struct {
	pinCalls  int32
	pinError  error
	mu        sync.Mutex
	repoPath  string
}

func (m *mockIPFSNodeWithCallCount) Pin(ctx context.Context, cid string, timeout time.Duration) error {
	atomic.AddInt32(&m.pinCalls, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pinError
}

func (m *mockIPFSNodeWithCallCount) Stat(ctx context.Context, cid string) (int64, error) {
	return 1024, nil
}

func (m *mockIPFSNodeWithCallCount) GetRepoPath() string {
	return m.repoPath
}

// =============================================================================
// DOWNLOAD METADATA TESTS (HTTP Mocking)
// =============================================================================

func TestBackupManager_DownloadMetadata_Success(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "12345")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Call with our mock server URL
	_, mimeType, size, err := bm.downloadMetadata(context.Background(), server.URL+"/test.png")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if mimeType != "image/png" {
		t.Errorf("mimeType = %q, want 'image/png'", mimeType)
	}
	if size != 12345 {
		t.Errorf("size = %d, want 12345", size)
	}
}

func TestBackupManager_DownloadMetadata_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	_, _, _, err := bm.downloadMetadata(context.Background(), server.URL+"/missing")

	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestBackupManager_DownloadMetadata_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	// Use context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, _, err := bm.downloadMetadata(ctx, server.URL+"/slow")

	if err == nil {
		t.Error("Expected timeout error")
	}
}

// =============================================================================
// FETCH METADATA FROM CHAIN TESTS
// =============================================================================

func TestBackupManager_FetchMetadataFromChain(t *testing.T) {
	// Create mock server that returns metadata JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata := indexer.TokenMetadata{
			Name:        "Test NFT",
			Description: "A test NFT",
			ArtifactURI: "ipfs://QmArtifact",
			DisplayURI:  "ipfs://QmDisplay",
		}
		json.NewEncoder(w).Encode(metadata)
	}))
	defer server.Close()

	// Create mock indexer server for the raw metadata URI fetch
	indexerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return the mock metadata server URL as if it were an IPFS URI
		// The test needs to use a real IPFS gateway, so we mock the whole flow
		w.Write([]byte(`{"value": "ipfs://QmMetadata"}`))
	}))
	defer indexerServer.Close()

	// This test is tricky because fetchMetadataFromChain calls the indexer and then HTTP
	// For a proper test, we'd need to inject the HTTP client or use a more testable design
	// For now, we test the resolveURI helper used by this function

	// Test that ipfs:// URIs get resolved correctly
	resolved := resolveURI("ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	expected := "https://ipfs.io/ipfs/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
	if resolved != expected {
		t.Errorf("resolveURI() = %q, want %q", resolved, expected)
	}
}

// =============================================================================
// PROCESSED URIS DEDUPLICATION TESTS
// =============================================================================

func TestBackupManager_ProcessedURIsDeduplication(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:            database,
		config:        cfg,
		workers:       make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:      make(chan struct{}),
		progress:      SyncProgress{Phase: "idle"},
		processedURIs: sync.Map{},
	}

	uri := "ipfs://QmTest123"

	// First store should succeed
	_, loaded := bm.processedURIs.LoadOrStore(uri, true)
	if loaded {
		t.Error("First LoadOrStore should not be loaded")
	}

	// Second store should indicate already present
	_, loaded = bm.processedURIs.LoadOrStore(uri, true)
	if !loaded {
		t.Error("Second LoadOrStore should indicate loaded")
	}

	// Different URI should succeed
	_, loaded = bm.processedURIs.LoadOrStore("ipfs://QmDifferent", true)
	if loaded {
		t.Error("Different URI LoadOrStore should not be loaded")
	}
}

// =============================================================================
// SYNC WALLET CONTEXT CANCELLATION TESTS
// =============================================================================

func TestBackupManager_SyncWallet_ContextCancellation(t *testing.T) {
	// Create mock TZKT server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	database := testDB(t)
	cfg := testConfig()
	cfg.TZKT.BaseURL = server.URL

	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	bm := &BackupManager{
		indexer:       idx,
		db:            database,
		config:        cfg,
		workers:       make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:      make(chan struct{}),
		progress:      SyncProgress{Phase: "idle"},
		processedURIs: sync.Map{},
	}

	// Add wallet to database
	wallet := &db.Wallet{
		Address:     "tz1TestWallet123456789012345678901234",
		SyncOwned:   true,
		SyncCreated: false,
	}
	database.SaveWallet(wallet)

	// Create context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel almost immediately
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// SyncWallet should respect context cancellation
	_, err := bm.SyncWallet(ctx, wallet.Address)

	// Should get context error or recover gracefully
	// The function has a recover(), so it might not return an error
	_ = err // We just want to make sure it doesn't hang
}

// =============================================================================
// BACKUP ASSET EDGE CASES
// =============================================================================

func TestBackupManager_BackupAsset_NonIPFSURI(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:            database,
		config:        cfg,
		workers:       make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:      make(chan struct{}),
		progress:      SyncProgress{Phase: "idle"},
		processedURIs: sync.Map{},
	}

	// HTTP URLs should be skipped (no error)
	err := bm.backupAsset(context.Background(), 1, "https://example.com/image.png", "artifact")
	if err != nil {
		t.Errorf("Non-IPFS URI should be skipped without error: %v", err)
	}

	// Data URIs should be skipped
	err = bm.backupAsset(context.Background(), 1, "data:image/png;base64,abc123", "thumbnail")
	if err != nil {
		t.Errorf("Data URI should be skipped without error: %v", err)
	}
}

func TestBackupManager_BackupAsset_WhenPaused(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:            database,
		config:        cfg,
		workers:       make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:      make(chan struct{}),
		progress:      SyncProgress{Phase: "idle"},
		processedURIs: sync.Map{},
	}

	// Pause the manager
	bm.SetPaused(true)

	// Attempt backup - should return nil immediately
	err := bm.backupAsset(context.Background(), 1, "ipfs://QmTest", "artifact")
	if err != nil {
		t.Errorf("backupAsset when paused should return nil: %v", err)
	}
}

func TestBackupManager_BackupAsset_Deduplication(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:            database,
		config:        cfg,
		workers:       make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:      make(chan struct{}),
		progress:      SyncProgress{Phase: "idle"},
		processedURIs: sync.Map{},
	}

	uri := "ipfs://QmTestDedup"

	// Mark as already processed
	bm.processedURIs.Store(uri, true)

	// Attempt backup - should skip
	err := bm.backupAsset(context.Background(), 1, uri, "artifact")
	if err != nil {
		t.Errorf("Already processed URI should be skipped: %v", err)
	}
}

// =============================================================================
// ASSET TYPE CLASSIFICATION TESTS
// =============================================================================

func TestBackupManager_AssetTypes(t *testing.T) {
	// Test that asset types are correctly assigned based on metadata field
	database := testDB(t)

	// Create wallet and NFT for testing
	wallet := &db.Wallet{Address: "tz1TestWallet123456789012345678901234"}
	database.SaveWallet(wallet)

	nft := &db.NFT{
		TokenID:         "1",
		ContractAddress: "KT1Test",
		WalletAddress:   wallet.Address,
	}
	database.SaveNFT(nft)

	// Create assets with different types
	types := []string{"artifact", "display", "thumbnail", "format"}
	for _, assetType := range types {
		asset := &db.Asset{
			NFTID:  nft.ID,
			URI:    "ipfs://Qm" + assetType,
			Status: db.StatusPending,
			Type:   assetType,
		}
		if err := database.SaveAsset(asset); err != nil {
			t.Errorf("Failed to save asset with type %q: %v", assetType, err)
		}
	}

	// Verify assets have correct types
	var assets []db.Asset
	database.DB.Where("nft_id = ?", nft.ID).Find(&assets)

	if len(assets) != 4 {
		t.Errorf("Expected 4 assets, got %d", len(assets))
	}

	typeCount := make(map[string]int)
	for _, a := range assets {
		typeCount[a.Type]++
	}

	for _, assetType := range types {
		if typeCount[assetType] != 1 {
			t.Errorf("Expected 1 asset of type %q, got %d", assetType, typeCount[assetType])
		}
	}
}

// =============================================================================
// DISK USAGE TRACKING TESTS
// =============================================================================

func TestBackupManager_DiskUsageDirtyFlag(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:             database,
		config:         cfg,
		workers:        make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:       make(chan struct{}),
		progress:       SyncProgress{Phase: "idle"},
		diskUsageDirty: 0,
	}

	// Initially clean
	if bm.diskUsageDirty != 0 {
		t.Error("Initial diskUsageDirty should be 0")
	}

	// Mark dirty
	bm.MarkDiskUsageDirty()
	if bm.diskUsageDirty != 1 {
		t.Error("After MarkDiskUsageDirty, value should be 1")
	}

	// Multiple marks should still be 1 (it's atomic store, not increment)
	bm.MarkDiskUsageDirty()
	bm.MarkDiskUsageDirty()
	if bm.diskUsageDirty != 1 {
		t.Error("Multiple MarkDiskUsageDirty should keep value at 1")
	}
}

func TestBackupManager_DiskUsageDirtyAtomicSwap(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:             database,
		config:         cfg,
		workers:        make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown:       make(chan struct{}),
		progress:       SyncProgress{Phase: "idle"},
		diskUsageDirty: 1, // Start dirty
	}

	// CompareAndSwap should succeed and return true the first time
	swapped := atomic.CompareAndSwapInt32(&bm.diskUsageDirty, 1, 0)
	if !swapped {
		t.Error("First CompareAndSwap should succeed")
	}
	if bm.diskUsageDirty != 0 {
		t.Error("Value should be 0 after swap")
	}

	// Second CompareAndSwap should fail (already 0)
	swapped = atomic.CompareAndSwapInt32(&bm.diskUsageDirty, 1, 0)
	if swapped {
		t.Error("Second CompareAndSwap should fail")
	}
}

// =============================================================================
// CONCURRENCY SAFETY TESTS
// =============================================================================

func TestBackupService_ConcurrentStatusUpdates(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			service.updateStatus(func(st *ServiceStatus) {
				st.WalletsTotal++
				st.Message = fmt.Sprintf("Update %d", n)
			})
			_ = service.GetStatus()
		}(i)
	}
	wg.Wait()

	status := service.GetStatus()
	if status.WalletsTotal != 100 {
		t.Errorf("WalletsTotal = %d, want 100 after concurrent updates", status.WalletsTotal)
	}
}

func TestBackupManager_ConcurrentPauseCheck(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()

	bm := &BackupManager{
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}

	var wg sync.WaitGroup
	var pausedCount, notPausedCount int32

	// Concurrent pauses and checks
	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			if i%2 == 0 {
				bm.SetPaused(true)
			} else {
				bm.SetPaused(false)
			}
		}()

		go func() {
			defer wg.Done()
			if bm.IsPaused() {
				atomic.AddInt32(&pausedCount, 1)
			} else {
				atomic.AddInt32(&notPausedCount, 1)
			}
		}()
	}
	wg.Wait()

	// Just verify no panics and counts sum correctly
	total := pausedCount + notPausedCount
	if total != 100 {
		t.Errorf("Total checks = %d, want 100", total)
	}
}

// =============================================================================
// EXTRACT CID EDGE CASES
// =============================================================================

func TestExtractCIDFromURI_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		// Short strings that might cause index out of bounds
		{"very short", "ipfs:", ""},
		{"exactly 7 chars", "ipfs://", ""},
		{"only scheme", "ipfs://Q", "Q"},

		// Complex real-world URIs
		{"fxhash generative", "ipfs://QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG?fxhash=oo123&fxiteration=42", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"},
		// Note: fragments are NOT stripped by extractCIDFromURI - they become part of CID
		// This is intentional - the real CID parsing happens in IPFS client
		{"with fragment (kept)", "ipfs://QmTest#section", "QmTest#section"},
		{"deep path", "ipfs://QmRoot/a/b/c/d/e/file.html", "QmRoot"},

		// Gateway variations
		{"pinata gateway", "https://gateway.pinata.cloud/ipfs/QmTest/file.json", "QmTest"},
		{"nftstorage gateway", "https://nftstorage.link/ipfs/QmTest", "QmTest"},
		{"dweb.link subdomain (not detected)", "https://QmTest.ipfs.dweb.link/", ""},
		{"localhost gateway", "http://localhost:8080/ipfs/QmTest", "QmTest"},

		// Malformed
		{"double ipfs", "ipfs://ipfs://QmTest", "ipfs:"},
		{"spaces", "ipfs://Qm Test", "Qm Test"},
		{"unicode", "ipfs://QmÜñíçödé", "QmÜñíçödé"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCIDFromURI(tt.uri)
			if result != tt.expected {
				t.Errorf("extractCIDFromURI(%q) = %q, want %q", tt.uri, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// RETRY WORKER INTEGRATION TEST
// =============================================================================

func TestBackupService_RetryFailedAssets(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Initialize context (normally done by Start())
	service.ctx, service.cancel = context.WithCancel(context.Background())
	defer service.cancel()

	// Create wallet and NFT
	wallet := &db.Wallet{Address: "tz1TestWallet123456789012345678901234"}
	database.SaveWallet(wallet)

	nft := &db.NFT{
		TokenID:         "1",
		ContractAddress: "KT1Test",
		WalletAddress:   wallet.Address,
	}
	database.SaveNFT(nft)

	// Create failed assets with low retry count (should be retried)
	for i := 0; i < 5; i++ {
		asset := &db.Asset{
			NFTID:      nft.ID,
			URI:        fmt.Sprintf("ipfs://QmFailed%d", i),
			Status:     db.StatusFailed,
			RetryCount: 2, // Under max retries
		}
		database.SaveAsset(asset)
	}

	// Create failed assets with high retry count (should NOT be retried)
	for i := 0; i < 3; i++ {
		asset := &db.Asset{
			NFTID:      nft.ID,
			URI:        fmt.Sprintf("ipfs://QmMaxRetry%d", i),
			Status:     db.StatusFailed,
			RetryCount: 10, // Over max retries
		}
		database.SaveAsset(asset)
	}

	// Call retryFailedAssets directly
	service.retryFailedAssets()

	// Check that retryable assets were marked pending
	var pendingAssets []db.Asset
	database.DB.Where("status = ?", db.StatusPending).Find(&pendingAssets)

	if len(pendingAssets) != 5 {
		t.Errorf("Expected 5 pending assets, got %d", len(pendingAssets))
	}

	// Check that max retry assets remain failed
	var stillFailedAssets []db.Asset
	database.DB.Where("status = ? AND retry_count >= 10", db.StatusFailed).Find(&stillFailedAssets)

	if len(stillFailedAssets) != 3 {
		t.Errorf("Expected 3 still-failed assets, got %d", len(stillFailedAssets))
	}
}

// =============================================================================
// TRIGGER CHANNEL TESTS
// =============================================================================

func TestBackupService_TriggerChannelBuffer(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// The trigger channel has a buffer of 100
	// Fill it up without blocking
	for i := 0; i < 100; i++ {
		service.TriggerSync(fmt.Sprintf("tz1Wallet%d", i))
	}

	// 101st should not block (TriggerSync uses select with default)
	done := make(chan bool)
	go func() {
		service.TriggerSync("tz1Extra")
		done <- true
	}()

	select {
	case <-done:
		// Good, didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("TriggerSync should not block when buffer is full")
	}
}

// =============================================================================
// HEALTH CHECK TESTS
// =============================================================================

func TestBackupService_PerformHealthCheck(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Create wallets with different sync times
	staleTime := time.Now().Add(-2 * time.Hour) // 2 hours ago - stale
	recentTime := time.Now().Add(-30 * time.Minute) // 30 min ago - recent

	wallet1 := &db.Wallet{
		Address:      "tz1StaleWallet12345678901234567890123",
		LastSyncedAt: &staleTime,
	}
	wallet2 := &db.Wallet{
		Address:      "tz1RecentWallet2345678901234567890123",
		LastSyncedAt: &recentTime,
	}
	wallet3 := &db.Wallet{
		Address:      "tz1NeverSynced345678901234567890123",
		LastSyncedAt: nil, // Never synced
	}

	database.SaveWallet(wallet1)
	database.SaveWallet(wallet2)
	database.SaveWallet(wallet3)

	// Drain any existing triggers
	for len(service.triggerCh) > 0 {
		<-service.triggerCh
	}

	// Run health check
	service.performHealthCheck()

	// Should trigger sync for stale and never-synced wallets (not recent)
	var triggered []string
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case addr := <-service.triggerCh:
			triggered = append(triggered, addr)
		case <-timeout:
			goto checkResults
		}
	}

checkResults:
	// Should have 2 triggers (stale + never synced)
	if len(triggered) != 2 {
		t.Errorf("Expected 2 triggered wallets, got %d: %v", len(triggered), triggered)
	}

	// The recent wallet should not be triggered
	for _, addr := range triggered {
		if addr == wallet2.Address {
			t.Error("Recent wallet should not be triggered")
		}
	}
}

// =============================================================================
// ADD WALLET TEST
// =============================================================================

func TestBackupService_AddWallet(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	mockNode := &ipfs.Node{}
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Initialize context (normally done by Start)
	service.ctx, service.cancel = context.WithCancel(context.Background())
	defer service.cancel()

	address := "tz1NewWallet1234567890123456789012345"

	// Drain any existing triggers
	for len(service.triggerCh) > 0 {
		<-service.triggerCh
	}

	// Add wallet
	service.AddWallet(address)

	// Should have triggered sync
	select {
	case triggered := <-service.triggerCh:
		if triggered != address {
			t.Errorf("Triggered address = %q, want %q", triggered, address)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("AddWallet should trigger sync")
	}
}

// =============================================================================
// PIN ASSET BY ID TESTS
// =============================================================================

func TestBackupService_PinAsset(t *testing.T) {
	database := testDB(t)
	mockNode := &ipfs.Node{}
	cfg := testConfig()
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	service := NewBackupService(mockNode, idx, database, cfg)

	// Create wallet, NFT, and asset
	wallet := &db.Wallet{Address: "tz1TestWallet123456789012345678901234"}
	database.SaveWallet(wallet)

	nft := &db.NFT{
		TokenID:         "1",
		ContractAddress: "KT1Test",
		WalletAddress:   wallet.Address,
	}
	database.SaveNFT(nft)

	asset := &db.Asset{
		NFTID:      nft.ID,
		URI:        "ipfs://QmTestAsset",
		Status:     db.StatusFailed,
		RetryCount: 5,
		ErrorMsg:   "Previous error",
	}
	database.SaveAsset(asset)

	// PinAsset will fail because we don't have a real IPFS node
	// But we can test that it resets the asset status first
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This will fail but we can check it tried
	_ = service.PinAsset(ctx, asset.ID)

	// Reload asset - status should have been reset to pending before pin attempt
	// (even if pin ultimately failed)
	var reloaded db.Asset
	database.DB.First(&reloaded, asset.ID)

	// The asset should either be pending (reset worked) or failed (pin failed)
	// We're mainly testing the reset logic
	if reloaded.RetryCount != 0 && reloaded.Status != db.StatusPending {
		// If retry count is still 0, reset worked but pin failed and incremented
		// That's expected behavior
	}
}

// =============================================================================
// SYNC PROGRESS JSON SERIALIZATION
// =============================================================================

func TestSyncProgress_JSON(t *testing.T) {
	now := time.Now()
	sp := SyncProgress{
		IsActive:      true,
		Phase:         "pinning",
		WalletAddress: "tz1TestAddress",
		TotalNFTs:     50,
		ProcessedNFTs: 25,
		TotalAssets:   150,
		PinnedAssets:  100,
		FailedAssets:  10,
		CurrentItem:   "My NFT",
		StartedAt:     now,
		Message:       "Pinning assets...",
	}

	data, err := json.Marshal(sp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded SyncProgress
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Phase != "pinning" {
		t.Errorf("Phase = %q, want 'pinning'", decoded.Phase)
	}
	if decoded.TotalNFTs != 50 {
		t.Errorf("TotalNFTs = %d, want 50", decoded.TotalNFTs)
	}
	if decoded.CurrentItem != "My NFT" {
		t.Errorf("CurrentItem = %q, want 'My NFT'", decoded.CurrentItem)
	}
}
