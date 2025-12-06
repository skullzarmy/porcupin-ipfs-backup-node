package db

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *Database {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := InitDB(db); err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return NewDatabase(db)
}

func TestInitDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	err = InitDB(db)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Verify tables exist by querying them
	var count int64
	if err := db.Model(&Wallet{}).Count(&count).Error; err != nil {
		t.Errorf("Wallet table not created: %v", err)
	}
	if err := db.Model(&NFT{}).Count(&count).Error; err != nil {
		t.Errorf("NFT table not created: %v", err)
	}
	if err := db.Model(&Asset{}).Count(&count).Error; err != nil {
		t.Errorf("Asset table not created: %v", err)
	}
	if err := db.Model(&Setting{}).Count(&count).Error; err != nil {
		t.Errorf("Setting table not created: %v", err)
	}
}

func TestWalletCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	wallet := &Wallet{
		Address:     "tz1TestAddress123",
		Alias:       "Test Wallet",
		Type:        "owned",
		SyncOwned:   true,
		SyncCreated: false,
		LastUpdated: time.Now(),
	}

	err := db.SaveWallet(wallet)
	if err != nil {
		t.Fatalf("SaveWallet failed: %v", err)
	}

	// Read
	retrieved, err := db.GetWallet("tz1TestAddress123")
	if err != nil {
		t.Fatalf("GetWallet failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetWallet returned nil")
	}
	if retrieved.Alias != "Test Wallet" {
		t.Errorf("Alias mismatch: got %q, want %q", retrieved.Alias, "Test Wallet")
	}
	if !retrieved.SyncOwned {
		t.Error("SyncOwned should be true")
	}
	// Note: SyncCreated defaults to true in GORM model, so even if we set false,
	// GORM treats false as zero value and applies the default
	if !retrieved.SyncCreated {
		t.Error("SyncCreated should default to true")
	}

	// Update
	wallet.Alias = "Updated Wallet"
	err = db.SaveWallet(wallet)
	if err != nil {
		t.Fatalf("SaveWallet (update) failed: %v", err)
	}

	retrieved, _ = db.GetWallet("tz1TestAddress123")
	if retrieved.Alias != "Updated Wallet" {
		t.Errorf("Update failed: got %q, want %q", retrieved.Alias, "Updated Wallet")
	}

	// Delete
	err = db.DeleteWallet("tz1TestAddress123")
	if err != nil {
		t.Fatalf("DeleteWallet failed: %v", err)
	}

	retrieved, _ = db.GetWallet("tz1TestAddress123")
	if retrieved != nil {
		t.Error("Wallet should be deleted")
	}
}

func TestGetAllWallets(t *testing.T) {
	db := setupTestDB(t)

	// Add multiple wallets
	wallets := []Wallet{
		{Address: "tz1Wallet1", Alias: "Wallet 1", LastUpdated: time.Now()},
		{Address: "tz1Wallet2", Alias: "Wallet 2", LastUpdated: time.Now()},
		{Address: "tz1Wallet3", Alias: "Wallet 3", LastUpdated: time.Now()},
	}

	for _, w := range wallets {
		if err := db.SaveWallet(&w); err != nil {
			t.Fatalf("SaveWallet failed: %v", err)
		}
	}

	// Get all
	all, err := db.GetAllWallets()
	if err != nil {
		t.Fatalf("GetAllWallets failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Expected 3 wallets, got %d", len(all))
	}
}

func TestNFTUpsert(t *testing.T) {
	db := setupTestDB(t)

	// First insert
	nft := &NFT{
		TokenID:         "123",
		ContractAddress: "KT1TestContract",
		WalletAddress:   "tz1Owner",
		Name:            "Test NFT",
		ArtifactURI:     "ipfs://QmTest",
	}

	err := db.SaveNFT(nft)
	if err != nil {
		t.Fatalf("SaveNFT failed: %v", err)
	}

	originalID := nft.ID
	if originalID == 0 {
		t.Error("NFT ID should be set after save")
	}

	// Upsert with same token_id + contract
	nft2 := &NFT{
		TokenID:         "123",
		ContractAddress: "KT1TestContract",
		WalletAddress:   "tz1Owner",
		Name:            "Updated NFT Name",
		ArtifactURI:     "ipfs://QmUpdated",
	}

	err = db.SaveNFT(nft2)
	if err != nil {
		t.Fatalf("SaveNFT (upsert) failed: %v", err)
	}

	// Should have same ID (upsert, not insert)
	if nft2.ID != originalID {
		t.Errorf("ID mismatch after upsert: got %d, want %d", nft2.ID, originalID)
	}

	// Verify only one NFT exists
	var count int64
	db.Model(&NFT{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 NFT, got %d", count)
	}
}

func TestAssetCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create NFT first (for foreign key)
	nft := &NFT{
		TokenID:         "1",
		ContractAddress: "KT1Test",
		WalletAddress:   "tz1Test",
		Name:            "Test",
	}
	db.SaveNFT(nft)

	// Create asset
	asset := &Asset{
		URI:    "ipfs://QmTestAsset",
		NFTID:  nft.ID,
		Type:   "artifact",
		Status: StatusPending,
	}

	err := db.SaveAsset(asset)
	if err != nil {
		t.Fatalf("SaveAsset failed: %v", err)
	}

	// Get by URI
	retrieved, err := db.GetAssetByURI("ipfs://QmTestAsset")
	if err != nil {
		t.Fatalf("GetAssetByURI failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Asset not found")
	}
	if retrieved.Status != StatusPending {
		t.Errorf("Status mismatch: got %q, want %q", retrieved.Status, StatusPending)
	}

	// Update status
	retrieved.Status = StatusPinned
	now := time.Now()
	retrieved.PinnedAt = &now
	retrieved.SizeBytes = 12345

	err = db.SaveAsset(retrieved)
	if err != nil {
		t.Fatalf("SaveAsset (update) failed: %v", err)
	}

	// Verify update
	retrieved, _ = db.GetAssetByURI("ipfs://QmTestAsset")
	if retrieved.Status != StatusPinned {
		t.Errorf("Status update failed: got %q", retrieved.Status)
	}
	if retrieved.SizeBytes != 12345 {
		t.Errorf("SizeBytes mismatch: got %d", retrieved.SizeBytes)
	}
}

func TestGetPendingAssets(t *testing.T) {
	db := setupTestDB(t)

	// Create NFT
	nft := &NFT{TokenID: "1", ContractAddress: "KT1", WalletAddress: "tz1"}
	db.SaveNFT(nft)

	// Create assets with different statuses
	assets := []Asset{
		{URI: "ipfs://Qm1", NFTID: nft.ID, Status: StatusPending},
		{URI: "ipfs://Qm2", NFTID: nft.ID, Status: StatusPending},
		{URI: "ipfs://Qm3", NFTID: nft.ID, Status: StatusPinned},
		{URI: "ipfs://Qm4", NFTID: nft.ID, Status: StatusFailed},
		{URI: "ipfs://Qm5", NFTID: nft.ID, Status: StatusPending},
	}

	for _, a := range assets {
		db.SaveAsset(&a)
	}

	// Get pending
	pending, err := db.GetPendingAssets(10)
	if err != nil {
		t.Fatalf("GetPendingAssets failed: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("Expected 3 pending assets, got %d", len(pending))
	}

	// Test limit
	pending, _ = db.GetPendingAssets(2)
	if len(pending) != 2 {
		t.Errorf("Limit not working: expected 2, got %d", len(pending))
	}
}

func TestGetRetryableAssets(t *testing.T) {
	db := setupTestDB(t)

	nft := &NFT{TokenID: "1", ContractAddress: "KT1", WalletAddress: "tz1"}
	db.SaveNFT(nft)

	assets := []Asset{
		{URI: "ipfs://Qm1", NFTID: nft.ID, Status: StatusFailed, RetryCount: 0},
		{URI: "ipfs://Qm2", NFTID: nft.ID, Status: StatusFailed, RetryCount: 2},
		{URI: "ipfs://Qm3", NFTID: nft.ID, Status: StatusFailed, RetryCount: 5}, // exceeds max
		{URI: "ipfs://Qm4", NFTID: nft.ID, Status: StatusFailedUnavailable, RetryCount: 1},
		{URI: "ipfs://Qm5", NFTID: nft.ID, Status: StatusPinned, RetryCount: 0}, // not failed
	}

	for _, a := range assets {
		db.SaveAsset(&a)
	}

	// Max 3 retries
	retryable, err := db.GetRetryableAssets(3, 10)
	if err != nil {
		t.Fatalf("GetRetryableAssets failed: %v", err)
	}
	if len(retryable) != 3 {
		t.Errorf("Expected 3 retryable assets, got %d", len(retryable))
	}

	// Verify ordering (by retry_count ASC)
	if retryable[0].RetryCount > retryable[1].RetryCount {
		t.Error("Results should be ordered by retry_count ASC")
	}
}

func TestGetAssetStats(t *testing.T) {
	db := setupTestDB(t)

	nft := &NFT{TokenID: "1", ContractAddress: "KT1", WalletAddress: "tz1"}
	db.SaveNFT(nft)

	assets := []Asset{
		{URI: "ipfs://Qm1", NFTID: nft.ID, Status: StatusPending, SizeBytes: 0},
		{URI: "ipfs://Qm2", NFTID: nft.ID, Status: StatusPinned, SizeBytes: 1000},
		{URI: "ipfs://Qm3", NFTID: nft.ID, Status: StatusPinned, SizeBytes: 2000},
		{URI: "ipfs://Qm4", NFTID: nft.ID, Status: StatusFailed, SizeBytes: 0},
		{URI: "ipfs://Qm5", NFTID: nft.ID, Status: StatusFailedUnavailable, SizeBytes: 0},
	}

	for _, a := range assets {
		db.SaveAsset(&a)
	}

	stats, err := db.GetAssetStats()
	if err != nil {
		t.Fatalf("GetAssetStats failed: %v", err)
	}

	if stats[StatusPending] != 1 {
		t.Errorf("Pending count: got %d, want 1", stats[StatusPending])
	}
	if stats[StatusPinned] != 2 {
		t.Errorf("Pinned count: got %d, want 2", stats[StatusPinned])
	}
	if stats[StatusFailed] != 1 {
		t.Errorf("Failed count: got %d, want 1", stats[StatusFailed])
	}
	if stats[StatusFailedUnavailable] != 1 {
		t.Errorf("FailedUnavailable count: got %d, want 1", stats[StatusFailedUnavailable])
	}
	if stats["total_size_bytes"] != 3000 {
		t.Errorf("Total size: got %d, want 3000", stats["total_size_bytes"])
	}
	if stats["nft_count"] != 1 {
		t.Errorf("NFT count: got %d, want 1", stats["nft_count"])
	}
}

func TestSettings(t *testing.T) {
	db := setupTestDB(t)

	// Get non-existent setting
	val, err := db.GetSetting("non_existent")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty string for non-existent setting, got %q", val)
	}

	// Set setting
	err = db.SetSetting("test_key", "test_value")
	if err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	// Get setting
	val, err = db.GetSetting("test_key")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Got %q, want %q", val, "test_value")
	}

	// Update setting
	err = db.SetSetting("test_key", "updated_value")
	if err != nil {
		t.Fatalf("SetSetting (update) failed: %v", err)
	}

	val, _ = db.GetSetting("test_key")
	if val != "updated_value" {
		t.Errorf("Update failed: got %q", val)
	}
}

func TestUpdateWalletSyncTime(t *testing.T) {
	db := setupTestDB(t)

	wallet := &Wallet{
		Address:     "tz1Test",
		LastUpdated: time.Now().Add(-time.Hour),
	}
	db.SaveWallet(wallet)

	// Update sync time
	err := db.UpdateWalletSyncTime("tz1Test", 12345)
	if err != nil {
		t.Fatalf("UpdateWalletSyncTime failed: %v", err)
	}

	// Verify
	updated, _ := db.GetWallet("tz1Test")
	if updated.LastSyncedLevel != 12345 {
		t.Errorf("LastSyncedLevel: got %d, want 12345", updated.LastSyncedLevel)
	}
	if updated.LastSyncedAt == nil {
		t.Error("LastSyncedAt should be set")
	}
}

func TestGetWalletNotFound(t *testing.T) {
	db := setupTestDB(t)

	wallet, err := db.GetWallet("tz1NonExistent")
	if err != nil {
		t.Fatalf("GetWallet should not error for not found: %v", err)
	}
	if wallet != nil {
		t.Error("Should return nil for non-existent wallet")
	}
}

func TestGetAssetByURINotFound(t *testing.T) {
	db := setupTestDB(t)

	asset, err := db.GetAssetByURI("ipfs://NonExistent")
	if err != nil {
		t.Fatalf("GetAssetByURI should not error for not found: %v", err)
	}
	if asset != nil {
		t.Error("Should return nil for non-existent asset")
	}
}
