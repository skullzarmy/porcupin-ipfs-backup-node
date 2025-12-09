package db

import (
	"time"

	"gorm.io/gorm"
)

// Asset status constants
const (
	StatusPending           = "pending"
	StatusPinned            = "pinned"
	StatusFailed            = "failed"
	StatusFailedUnavailable = "failed_unavailable"
)

// Database wraps gorm.DB with additional helper methods
type Database struct {
	*gorm.DB
}

// NewDatabase creates a new Database instance
func NewDatabase(db *gorm.DB) *Database {
	return &Database{DB: db}
}

// Wallet represents a Tezos wallet being tracked
type Wallet struct {
	Address         string     `gorm:"primaryKey" json:"address"`
	Alias           string     `json:"alias"`
	Type            string     `json:"type"` // "owned" or "created"
	SyncOwned       bool       `json:"sync_owned" gorm:"default:true"`   // Whether to sync owned NFTs
	SyncCreated     bool       `json:"sync_created" gorm:"default:true"` // Whether to sync created NFTs
	LastSyncedAt    *time.Time `json:"last_synced_at"`    // When we last fully synced this wallet
	LastSyncedLevel int64      `json:"last_synced_level"` // Blockchain level at last sync
	LastUpdated     time.Time  `json:"last_updated"`
	NFTs            []NFT      `gorm:"foreignKey:WalletAddress" json:"nfts,omitempty"`
}

// NFT represents a unique token on the Tezos blockchain
type NFT struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TokenID         string    `gorm:"uniqueIndex:idx_token_contract" json:"token_id"`
	ContractAddress string    `gorm:"uniqueIndex:idx_token_contract" json:"contract_address"`
	WalletAddress   string    `gorm:"index" json:"wallet_address"`
	Name            string    `json:"name"`          // Token name from metadata
	Description     string    `json:"description"`   // Token description
	CreatorAddress  string    `json:"creator"`       // First minter address
	ArtifactURI     string    `json:"artifact_uri"`
	DisplayURI      string    `json:"display_uri"`   // Often a smaller preview
	ThumbnailURI    string    `json:"thumbnail_uri"`
	RawMetadata     string    `json:"raw_metadata"` // JSON string
	Assets          []Asset   `gorm:"foreignKey:NFTID" json:"assets,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Asset represents a file on IPFS that needs to be pinned
type Asset struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	URI        string     `gorm:"uniqueIndex" json:"uri"` // ipfs://...
	NFTID      uint64     `gorm:"index" json:"nft_id"`
	NFT        *NFT       `gorm:"foreignKey:NFTID" json:"nft,omitempty"` // Relationship for joins
	Type       string     `json:"type"`      // "artifact", "thumbnail", "format", "metadata"
	MimeType   string     `json:"mime_type"` // e.g. "image/png"
	Status     string     `gorm:"index" json:"status"` // "pending", "pinned", "failed", "failed_unavailable"
	ErrorMsg   string     `json:"error_msg"` // Last error message if failed
	SizeBytes  int64      `json:"size_bytes"`
	RetryCount int        `json:"retry_count"`
	CreatedAt  time.Time  `json:"created_at"`
	PinnedAt   *time.Time `json:"pinned_at"`
}

// Setting stores key-value configuration/state
type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

// InitDB initializes the database and performs auto-migration
func InitDB(db *gorm.DB) error {
	return db.AutoMigrate(&Wallet{}, &NFT{}, &Asset{}, &Setting{})
}

// GetSetting retrieves a setting value by key
func (d *Database) GetSetting(key string) (string, error) {
	var setting Setting
	err := d.Where("key = ?", key).First(&setting).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return setting.Value, nil
}

// SetSetting saves a setting value
func (d *Database) SetSetting(key, value string) error {
	return d.Save(&Setting{Key: key, Value: value}).Error
}

// SaveNFT saves or updates an NFT (upsert by token_id + contract_address)
func (d *Database) SaveNFT(nft *NFT) error {
	// First try to find existing NFT
	var existing NFT
	err := d.Where("token_id = ? AND contract_address = ?", nft.TokenID, nft.ContractAddress).First(&existing).Error
	if err == nil {
		// Found existing - update it
		nft.ID = existing.ID
		nft.CreatedAt = existing.CreatedAt
	}
	return d.Save(nft).Error
}

// GetAssetByURI retrieves an asset by its URI
func (d *Database) GetAssetByURI(uri string) (*Asset, error) {
	var asset Asset
	err := d.Where("uri = ?", uri).First(&asset).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &asset, nil
}

// SaveAsset saves or updates an asset
func (d *Database) SaveAsset(asset *Asset) error {
	return d.Save(asset).Error
}

// GetWallet retrieves a wallet by address
func (d *Database) GetWallet(address string) (*Wallet, error) {
	var wallet Wallet
	err := d.Where("address = ?", address).First(&wallet).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &wallet, nil
}

// SaveWallet saves or updates a wallet
func (d *Database) SaveWallet(wallet *Wallet) error {
	return d.Save(wallet).Error
}

// GetPendingAssets retrieves all assets with pending status
// If limit is 0 or negative, returns all pending assets
func (d *Database) GetPendingAssets(limit int) ([]Asset, error) {
	var assets []Asset
	query := d.Where("status = ?", StatusPending)
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&assets).Error
	return assets, err
}

// GetAssetStats returns statistics about assets
func (d *Database) GetAssetStats() (map[string]int64, error) {
	stats := make(map[string]int64)
	
	// Count by status
	statuses := []string{StatusPending, StatusPinned, StatusFailed, StatusFailedUnavailable}
	for _, status := range statuses {
		var count int64
		if err := d.Model(&Asset{}).Where("status = ?", status).Count(&count).Error; err != nil {
			return nil, err
		}
		stats[status] = count
	}
	
	// Total size of pinned assets
	var totalSize int64
	if err := d.Model(&Asset{}).Where("status = ?", StatusPinned).Select("COALESCE(SUM(size_bytes), 0)").Scan(&totalSize).Error; err != nil {
		return nil, err
	}
	stats["total_size_bytes"] = totalSize
	
	// Count NFTs
	var nftCount int64
	if err := d.Model(&NFT{}).Count(&nftCount).Error; err != nil {
		return nil, err
	}
	stats["nft_count"] = nftCount
	
	return stats, nil
}

// GetAllWallets retrieves all wallets
func (d *Database) GetAllWallets() ([]Wallet, error) {
	var wallets []Wallet
	err := d.Find(&wallets).Error
	return wallets, err
}

// DeleteWallet removes a wallet by address
func (d *Database) DeleteWallet(address string) error {
	return d.Where("address = ?", address).Delete(&Wallet{}).Error
}

// GetRetryableAssets gets failed assets that can be retried
func (d *Database) GetRetryableAssets(maxRetries int, limit int) ([]Asset, error) {
	var assets []Asset
	err := d.Where("status IN (?, ?) AND retry_count < ?", 
		StatusFailed, StatusFailedUnavailable, maxRetries).
		Order("retry_count ASC, created_at ASC").
		Limit(limit).
		Find(&assets).Error
	return assets, err
}

// UpdateWalletSyncTime updates the last synced time and level for a wallet
func (d *Database) UpdateWalletSyncTime(address string, level int64) error {
	now := time.Now()
	return d.Model(&Wallet{}).Where("address = ?", address).Updates(map[string]interface{}{
		"last_synced_at":    now,
		"last_synced_level": level,
		"last_updated":      now,
	}).Error
}

