package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"porcupin/backend/api"
	"porcupin/backend/config"
	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
	"porcupin/backend/storage"
	"porcupin/backend/version"

	"github.com/glebarez/sqlite"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"
)

// App struct
type App struct {
	ctx           context.Context
	config        *config.Config
	database      *db.Database
	ipfsNode      *ipfs.Node
	indexer       *indexer.Indexer
	backupService *core.BackupService
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("Porcupin starting up...")

	// Setup data directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home dir: %v", err)
	}
	dataDir := filepath.Join(homeDir, ".porcupin")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data dir: %v", err)
	}

	// Load configuration
	configPath := filepath.Join(dataDir, "config.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
		// Ensure IPFS path is absolute if default
		if cfg.IPFS.RepoPath == "~/.porcupin/ipfs" {
			cfg.IPFS.RepoPath = filepath.Join(dataDir, "ipfs")
		}
	}
	a.config = cfg

	// Initialize database
	dbPath := filepath.Join(dataDir, "porcupin.db")
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err := db.InitDB(gormDB); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	a.database = db.NewDatabase(gormDB)
	log.Println("Database initialized")

	// Initialize IPFS node
	// Ensure repo path is absolute
	repoPath := cfg.IPFS.RepoPath
	if strings.HasPrefix(repoPath, "~/") {
		repoPath = filepath.Join(homeDir, repoPath[2:])
	}
	
	ipfsNode, err := ipfs.NewNode(repoPath, cfg.IPFS.SwarmPort)
	if err != nil {
		log.Fatalf("Failed to create IPFS node: %v", err)
	}

	if err := ipfsNode.Start(ctx); err != nil {
		log.Fatalf("Failed to start IPFS node: %v", err)
	}

	a.ipfsNode = ipfsNode
	log.Println("IPFS node started")

	// Initialize indexer
	a.indexer = indexer.NewIndexer(cfg.TZKT.BaseURL)
	log.Println("Indexer initialized")

	// Initialize backup service (handles automatic syncing)
	a.backupService = core.NewBackupService(ipfsNode, a.indexer, a.database, cfg)
	log.Println("Backup service initialized")
	
	// Initialize disk usage in background (don't block startup)
	go func() {
		a.backupService.GetManager().MarkDiskUsageDirty()
		a.backupService.GetManager().UpdateDiskUsage()
	}()
	
	// Start the automatic backup service
	a.backupService.Start(ctx)
	log.Println("Backup service started - auto-syncing enabled")

	log.Println("Porcupin startup complete!")
}

// shutdown is called during application termination
func (a *App) shutdown(ctx context.Context) {
	log.Println("Porcupin shutting down...")

	if a.backupService != nil {
		a.backupService.Stop()
	}

	if a.ipfsNode != nil {
		if err := a.ipfsNode.Stop(); err != nil {
			log.Printf("Error stopping IPFS node: %v", err)
		}
	}

	log.Println("Porcupin shutdown complete")
}

// domReady is called after front-end resources have been loaded
func (a *App) domReady(ctx context.Context) {
	// Show and focus the window
	wailsRuntime.WindowShow(ctx)
	wailsRuntime.WindowUnminimise(ctx)
	wailsRuntime.Show(ctx)
}

// beforeClose is called when the application is about to quit
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

// tezosAddressPattern validates Tezos wallet addresses (tz1, tz2, tz3, KT1)
var tezosAddressPattern = regexp.MustCompile(`^(tz[1-3]|KT1)[a-zA-Z0-9]{33}$`)

// AddWallet adds a wallet to be tracked
func (a *App) AddWallet(address string, alias string) error {
	// Validate Tezos address format
	if !tezosAddressPattern.MatchString(address) {
		return fmt.Errorf("invalid Tezos address format (expected tz1/tz2/tz3/KT1 followed by 33 alphanumeric characters)")
	}

	// Use global defaults for sync settings
	wallet := &db.Wallet{
		Address:     address,
		Alias:       alias,
		SyncOwned:   a.config.Backup.SyncOwned,
		SyncCreated: a.config.Backup.SyncCreated,
	}

	if err := a.database.SaveWallet(wallet); err != nil {
		return fmt.Errorf("failed to save wallet: %w", err)
	}
	
	// Notify backup service to start watching and sync this wallet
	a.backupService.AddWallet(address)

	return nil
}

// UpdateWalletSettings updates the sync settings for a specific wallet
func (a *App) UpdateWalletSettings(address string, syncOwned bool, syncCreated bool) error {
	return a.database.Model(&db.Wallet{}).Where("address = ?", address).Updates(map[string]interface{}{
		"sync_owned":   syncOwned,
		"sync_created": syncCreated,
	}).Error
}

// UpdateWalletAlias updates the alias for a specific wallet
func (a *App) UpdateWalletAlias(address string, alias string) error {
	return a.database.Model(&db.Wallet{}).Where("address = ?", address).Update("alias", alias).Error
}

// DeleteWallet removes a wallet and optionally its associated data (DB only, no unpin)
func (a *App) DeleteWallet(address string, deleteData bool) error {
	if deleteData {
		// Delete assets first (foreign key constraint)
		if err := a.database.DeleteAssetsByWallet(address); err != nil {
			return fmt.Errorf("failed to delete assets: %w", err)
		}
		// Delete NFTs
		if err := a.database.DeleteNFTsByWallet(address); err != nil {
			return fmt.Errorf("failed to delete NFTs: %w", err)
		}
	}
	// Delete the wallet record
	if err := a.database.DeleteWallet(address); err != nil {
		return fmt.Errorf("failed to delete wallet: %w", err)
	}
	return nil
}

// DeleteWalletWithUnpin removes a wallet and unpins all its assets from IPFS
func (a *App) DeleteWalletWithUnpin(address string) error {
	// Get all assets for this wallet
	assets, err := a.database.GetAssetsByWallet(address)
	if err != nil {
		return fmt.Errorf("failed to get assets: %w", err)
	}

	// Unpin each asset
	ctx := context.Background()
	for _, asset := range assets {
		cid := core.ExtractCIDFromURI(asset.URI)
		if cid == "" {
			continue
		}
		// Ignore errors - asset may not be pinned or may have already been unpinned
		_ = a.ipfsNode.Unpin(ctx, cid)
	}

	// Delete from database
	if err := a.database.DeleteAssetsByWallet(address); err != nil {
		return fmt.Errorf("failed to delete assets: %w", err)
	}
	if err := a.database.DeleteNFTsByWallet(address); err != nil {
		return fmt.Errorf("failed to delete NFTs: %w", err)
	}
	if err := a.database.DeleteWallet(address); err != nil {
		return fmt.Errorf("failed to delete wallet: %w", err)
	}

	// Mark disk usage dirty so it recalculates
	a.backupService.GetManager().MarkDiskUsageDirty()

	return nil
}

// SyncWallet synchronizes NFTs for a given wallet (manual trigger)
func (a *App) SyncWallet(address string) error {
	a.backupService.TriggerSync(address)
	return nil
}

// GetSyncProgress returns the current sync progress
func (a *App) GetSyncProgress() core.ServiceStatus {
	return a.backupService.GetStatus()
}

// PauseBackup pauses the automatic backup service
func (a *App) PauseBackup() {
	a.backupService.Pause()
}

// ResumeBackup resumes the automatic backup service
func (a *App) ResumeBackup() {
	a.backupService.Resume()
}

// IsBackupPaused returns whether the backup service is paused
func (a *App) IsBackupPaused() bool {
	return a.backupService.IsPaused()
}

// GetVersion returns the current version of Porcupin
func (a *App) GetVersion() string {
	return version.Version
}

// DiscoverServers scans the local network for Porcupin servers via mDNS
func (a *App) DiscoverServers() ([]api.DiscoveredServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return api.DiscoverServers(ctx, 5*time.Second)
}

// RemoteServerConfig holds configuration for connecting to a remote server
type RemoteServerConfig struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Token  string `json:"token"`
	UseTLS bool   `json:"useTLS"`
}

// RemoteHealthResponse holds the health check response from a remote server
type RemoteHealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

// TestRemoteConnection tests connectivity to a remote Porcupin server
func (a *App) TestRemoteConnection(cfg RemoteServerConfig) (*RemoteHealthResponse, error) {
	log.Printf("TestRemoteConnection: Connecting to %s:%d (TLS: %v)", cfg.Host, cfg.Port, cfg.UseTLS)
	
	client := api.NewRemoteClient(cfg.Host, cfg.Port, cfg.Token, cfg.UseTLS)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	health, err := client.Health(ctx)
	if err != nil {
		log.Printf("TestRemoteConnection: Failed - %v", err)
		return nil, err
	}
	
	log.Printf("TestRemoteConnection: Success - server version %s", health.Version)
	return &RemoteHealthResponse{
		Status:    health.Status,
		Version:   health.Version,
		Timestamp: health.Timestamp,
	}, nil
}

// RemoteProxyRequest holds a generic HTTP request to proxy to a remote server
type RemoteProxyRequest struct {
	Host    string            `json:"host"`
	Port    int               `json:"port"`
	Token   string            `json:"token"`
	UseTLS  bool              `json:"useTLS"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// RemoteProxyResponse holds the response from a proxied request
type RemoteProxyResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// RemoteProxy proxies an HTTP request to a remote Porcupin server
// This allows the frontend to make any API call to a remote server via Go
func (a *App) RemoteProxy(req RemoteProxyRequest) (*RemoteProxyResponse, error) {
	log.Printf("RemoteProxy: %s %s to %s:%d", req.Method, req.Path, req.Host, req.Port)
	
	client := api.NewRemoteClient(req.Host, req.Port, req.Token, req.UseTLS)
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	proxyReq := api.ProxyRequest{
		Method:  req.Method,
		Path:    req.Path,
		Headers: req.Headers,
		Body:    req.Body,
	}
	
	resp, err := client.Proxy(ctx, proxyReq)
	if err != nil {
		log.Printf("RemoteProxy: Failed - %v", err)
		return nil, err
	}
	
	log.Printf("RemoteProxy: Response status %d", resp.StatusCode)
	return &RemoteProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       resp.Body,
	}, nil
}

// GetWallets retrieves all tracked wallets
func (a *App) GetWallets() ([]db.Wallet, error) {
	var wallets []db.Wallet
	if err := a.database.Find(&wallets).Error; err != nil {
		return nil, err
	}
	return wallets, nil
}

// GetAssetStats returns asset statistics for the dashboard
func (a *App) GetAssetStats() (map[string]int64, error) {
	stats, err := a.database.GetAssetStats()
	if err != nil {
		return nil, err
	}
	
	// Get cached disk usage from DB (updated on pin/unpin/migration)
	diskUsageStr, _ := a.database.GetSetting("disk_usage_bytes")
	if diskUsageStr != "" {
		var diskUsage int64
		fmt.Sscanf(diskUsageStr, "%d", &diskUsage)
		stats["disk_usage_bytes"] = diskUsage
	}
	
	return stats, nil
}

// GetConfig returns the current configuration
func (a *App) GetConfig() config.Config {
	return *a.config
}

// GetAssets returns a paginated list of assets with their associated NFT info
func (a *App) GetAssets(page int, limit int, status string, search string) ([]db.Asset, error) {
	var assets []db.Asset
	offset := (page - 1) * limit
	
	query := a.database.DB.Model(&db.Asset{}).Preload("NFT")
	
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		likeSearch := "%" + search + "%"
		// Join with NFT table for searching by NFT name/description
		query = query.Joins("LEFT JOIN nfts ON nfts.id = assets.nft_id").
			Where("assets.type LIKE ? OR assets.mime_type LIKE ? OR assets.uri LIKE ? OR nfts.name LIKE ? OR nfts.description LIKE ?", 
				likeSearch, likeSearch, likeSearch, likeSearch, likeSearch)
	}
	
	err := query.Order("assets.id desc").Offset(offset).Limit(limit).Find(&assets).Error
	if err != nil {
		log.Printf("GetAssets error: %v", err)
		return nil, err
	}
	
	log.Printf("GetAssets fetched %d assets (page %d, limit %d, status %s, search %s)", len(assets), page, limit, status, search)
	return assets, nil
}

// GetRecentActivity returns the most recently pinned assets
func (a *App) GetRecentActivity(limit int) ([]db.Asset, error) {
	var assets []db.Asset
	err := a.database.DB.Model(&db.Asset{}).
		Preload("NFT").
		Where("status = ? AND pinned_at IS NOT NULL", db.StatusPinned).
		Order("pinned_at desc").
		Limit(limit).
		Find(&assets).Error
	if err != nil {
		return nil, err
	}
	return assets, nil
}

// GetNFTsWithAssets returns a paginated list of NFTs with their associated assets
func (a *App) GetNFTsWithAssets(page int, limit int, status string, search string) ([]db.NFT, error) {
	var nfts []db.NFT
	offset := (page - 1) * limit
	
	query := a.database.DB.Model(&db.NFT{}).Preload("Assets")

	// If filtering by asset status, we need to join/filter
	// This is tricky with GORM Preload + Pagination. 
	// Easier to find matching NFT IDs first if filters are present.
	if status != "" && status != "all" || search != "" {
		subQuery := a.database.DB.Model(&db.NFT{}).Select("DISTINCT nfts.id").
			Joins("LEFT JOIN assets ON assets.nft_id = nfts.id")
		
		if status != "" && status != "all" {
			subQuery = subQuery.Where("assets.status = ?", status)
		}
		
		if search != "" {
			likeSearch := "%" + search + "%"
			subQuery = subQuery.Where("nfts.name LIKE ? OR nfts.description LIKE ? OR assets.type LIKE ? OR assets.mime_type LIKE ? OR assets.uri LIKE ?", 
				likeSearch, likeSearch, likeSearch, likeSearch, likeSearch)
		}
		
		query = query.Where("id IN (?)", subQuery)
	}
	
	err := query.Order("id desc").
		Offset(offset).
		Limit(limit).
		Find(&nfts).Error
	
	if err != nil {
		log.Printf("GetNFTsWithAssets error: %v", err)
		return nil, err
	}
	
	log.Printf("GetNFTsWithAssets fetched %d NFTs (page %d, limit %d, status %s, search %s)", len(nfts), page, limit, status, search)
	return nfts, nil
}

// RetryAsset retries a failed asset by immediately pinning it
func (a *App) RetryAsset(assetID uint64) error {
	// Use the backup service to immediately pin the asset
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	
	return a.backupService.PinAsset(ctx, assetID)
}

// RetryAllFailed retries all failed assets
func (a *App) RetryAllFailed() (int64, error) {
	result := a.database.DB.Model(&db.Asset{}).
		Where("status IN (?, ?)", db.StatusFailed, db.StatusFailedUnavailable).
		Updates(map[string]interface{}{
			"status":      db.StatusPending,
			"retry_count": 0,
			"error_msg":   "",
		})
	return result.RowsAffected, result.Error
}

// ClearFailed removes all failed assets from the database
func (a *App) ClearFailed() (int64, error) {
	result := a.database.DB.Where("status IN (?, ?)", db.StatusFailed, db.StatusFailedUnavailable).Delete(&db.Asset{})
	return result.RowsAffected, result.Error
}

// GetFailedAssets returns all failed assets with their NFT info
func (a *App) GetFailedAssets() ([]db.Asset, error) {
	var assets []db.Asset
	err := a.database.DB.
		Where("status IN (?, ?)", db.StatusFailed, db.StatusFailedUnavailable).
		Preload("NFT").
		Order("id desc").
		Find(&assets).Error
	return assets, err
}

// GetStatus returns the current status of the application
func (a *App) GetStatus() map[string]interface{} {
	stats, _ := a.database.GetAssetStats()
	wallets, _ := a.GetWallets()
	
	return map[string]interface{}{
		"running":       true,
		"wallets_count": len(wallets),
		"pinned_count":  stats["pinned"],
		"pending_count": stats["pending"],
		"failed_count":  stats["failed"],
	}
}

// UnpinAsset unpins an asset from IPFS and updates its status
func (a *App) UnpinAsset(assetID uint64) error {
	var asset db.Asset
	if err := a.database.DB.First(&asset, assetID).Error; err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	// Extract CID from URI
	cid := extractCIDFromURI(asset.URI)
	if cid == "" {
		return fmt.Errorf("could not extract CID from URI: %s", asset.URI)
	}

	// Unpin from IPFS
	if err := a.ipfsNode.Unpin(a.ctx, cid); err != nil {
		log.Printf("Warning: unpin failed (may not be pinned): %v", err)
	}

	// Update status to pending (unpinned)
	asset.Status = db.StatusPending
	asset.PinnedAt = nil
	return a.database.SaveAsset(&asset)
}

// RepinAsset re-pins an unpinned asset
func (a *App) RepinAsset(assetID uint64) error {
	var asset db.Asset
	if err := a.database.DB.First(&asset, assetID).Error; err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	// Reset to pending so the backup manager will pick it up
	asset.Status = db.StatusPending
	asset.RetryCount = 0
	return a.database.SaveAsset(&asset)
}

// DeleteAsset removes an asset from the database and unpins it
func (a *App) DeleteAsset(assetID uint64) error {
	var asset db.Asset
	if err := a.database.DB.First(&asset, assetID).Error; err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	// Extract CID and unpin
	cid := extractCIDFromURI(asset.URI)
	if cid != "" {
		if err := a.ipfsNode.Unpin(a.ctx, cid); err != nil {
			log.Printf("Warning: unpin failed during delete: %v", err)
		}
	}

	// Delete from database
	return a.database.DB.Delete(&asset).Error
}

// ResyncAsset forces a re-sync of the NFT associated with this asset
func (a *App) ResyncAsset(assetID uint64) error {
	var asset db.Asset
	if err := a.database.DB.Preload("NFT").First(&asset, assetID).Error; err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	if asset.NFT == nil {
		return fmt.Errorf("no NFT associated with asset")
	}

	// Trigger sync for the wallet that owns this NFT
	a.backupService.TriggerSync(asset.NFT.WalletAddress)
	return nil
}

// ShowInFinder opens the IPFS blocks directory in the system file explorer
func (a *App) ShowInFinder() error {
	blocksPath := filepath.Join(a.ipfsNode.GetRepoPath(), "blocks")
	
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", blocksPath)
	case "windows":
		cmd = exec.Command("explorer", blocksPath)
	case "linux":
		cmd = exec.Command("xdg-open", blocksPath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	
	return cmd.Start()
}

// GetIPFSRepoPath returns the path to the IPFS repository
func (a *App) GetIPFSRepoPath() string {
	return a.ipfsNode.GetRepoPath()
}

// StorageInfo represents storage usage information
type StorageInfo struct {
	UsedBytes       int64   `json:"used_bytes"`        // From database (sum of asset sizes)
	UsedGB          float64 `json:"used_gb"`           // From database
	DiskUsageBytes  int64   `json:"disk_usage_bytes"`  // Actual IPFS repo size on disk
	DiskUsageGB     float64 `json:"disk_usage_gb"`     // Actual IPFS repo size on disk
	MaxStorageGB    int     `json:"max_storage_gb"`
	WarningPct      int     `json:"warning_pct"`
	UsagePct        float64 `json:"usage_pct"`
	IsWarning       bool    `json:"is_warning"`
	IsLimitReached  bool    `json:"is_limit_reached"`
	FreeDiskSpaceGB float64 `json:"free_disk_space_gb"`
	RepoPath        string  `json:"repo_path"`
}

// ClearDataStatus represents the progress of clearing all data
type ClearDataStatus struct {
	InProgress    bool   `json:"in_progress"`
	Phase         string `json:"phase"` // "unpinning", "garbage_collect", "clearing_db", "complete", "error"
	Message       string `json:"message"`
	TotalPins     int    `json:"total_pins"`
	UnpinnedCount int    `json:"unpinned_count"`
	Error         string `json:"error,omitempty"`
}

// GetStorageInfo returns current storage usage information
func (a *App) GetStorageInfo() (StorageInfo, error) {
	info := StorageInfo{
		MaxStorageGB: a.config.Backup.MaxStorageGB,
		WarningPct:   a.config.Backup.StorageWarningPct,
		RepoPath:     a.ipfsNode.GetRepoPath(),
	}

	// Get total size of pinned assets from database
	stats, err := a.database.GetAssetStats()
	if err != nil {
		return info, err
	}
	info.UsedBytes = stats["total_size_bytes"]
	info.UsedGB = float64(info.UsedBytes) / (1024 * 1024 * 1024)

	// Get cached disk usage from DB (updated on pin/unpin/migration)
	diskUsageStr, _ := a.database.GetSetting("disk_usage_bytes")
	if diskUsageStr != "" {
		var diskUsage int64
		fmt.Sscanf(diskUsageStr, "%d", &diskUsage)
		info.DiskUsageBytes = diskUsage
		info.DiskUsageGB = float64(diskUsage) / (1024 * 1024 * 1024)
	}

	// Calculate usage percentage if max is set (use disk usage for accuracy)
	if info.MaxStorageGB > 0 {
		info.UsagePct = (info.DiskUsageGB / float64(info.MaxStorageGB)) * 100
		info.IsWarning = info.UsagePct >= float64(info.WarningPct)
		info.IsLimitReached = info.UsagePct >= 100
	}

	// Get free disk space
	info.FreeDiskSpaceGB = getFreeDiskSpaceGB()

	return info, nil
}

// UpdateSettings updates the application settings
func (a *App) UpdateSettings(settings map[string]interface{}) error {
	// Update config values
	if v, ok := settings["max_storage_gb"].(float64); ok {
		a.config.Backup.MaxStorageGB = int(v)
	}
	if v, ok := settings["storage_warning_pct"].(float64); ok {
		a.config.Backup.StorageWarningPct = int(v)
	}
	if v, ok := settings["max_concurrency"].(float64); ok {
		a.config.Backup.MaxConcurrency = int(v)
	}
	if v, ok := settings["min_free_disk_space_gb"].(float64); ok {
		a.config.Backup.MinFreeDiskSpaceGB = int(v)
	}
	if v, ok := settings["max_file_size_gb"].(float64); ok {
		a.config.IPFS.MaxFileSize = int64(v * 1024 * 1024 * 1024)
	}
	if v, ok := settings["pin_timeout_minutes"].(float64); ok {
		a.config.IPFS.PinTimeout = time.Duration(v) * time.Minute
	}
	if v, ok := settings["sync_owned"].(bool); ok {
		a.config.Backup.SyncOwned = v
	}
	if v, ok := settings["sync_created"].(bool); ok {
		a.config.Backup.SyncCreated = v
	}
	// Note: ipfs_swarm_port is saved but requires app restart to take effect
	if v, ok := settings["ipfs_swarm_port"].(float64); ok {
		port := int(v)
		if port >= 1024 && port <= 65535 {
			a.config.IPFS.SwarmPort = port
		}
	}

	// Save config to file
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".porcupin", "config.yaml")
	return a.config.SaveConfig(configPath)
}

// RecoverMissingAssets triggers the verification and repair process for missing asset records
func (a *App) RecoverMissingAssets() (map[string]int, error) {
	return a.backupService.VerifyAndFixPins()
}

// ResetDatabase clears all NFTs, assets, and unpins all IPFS content
func (a *App) ResetDatabase() error {
	log.Println("Starting full data reset...")
	
	// Emit starting event
	wailsRuntime.EventsEmit(a.ctx, "clear:start", ClearDataStatus{
		InProgress: true,
		Phase:      "unpinning",
		Message:    "Unpinning IPFS content...",
	})
	
	// Step 1: Unpin all content from IPFS
	log.Println("Unpinning all IPFS content...")
	unpinned, err := a.ipfsNode.UnpinAll(a.ctx, func(total, current int) {
		wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
			InProgress:    true,
			Phase:         "unpinning",
			Message:       fmt.Sprintf("Unpinning content... %d/%d", current, total),
			TotalPins:     total,
			UnpinnedCount: current,
		})
	})
	if err != nil {
		log.Printf("Warning: failed to unpin all: %v", err)
		wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
			InProgress: true,
			Phase:      "unpinning",
			Message:    fmt.Sprintf("Warning: %v", err),
		})
	} else {
		log.Printf("Unpinned %d items", unpinned)
	}
	
	// Step 2: Run garbage collection to free disk space
	wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
		InProgress: true,
		Phase:      "garbage_collect",
		Message:    "Running garbage collection to free disk space...",
	})
	log.Println("Running IPFS garbage collection...")
	if err := a.ipfsNode.GarbageCollect(a.ctx); err != nil {
		log.Printf("Warning: garbage collection failed: %v", err)
	}
	
	// Step 3: Delete database records
	wailsRuntime.EventsEmit(a.ctx, "clear:progress", ClearDataStatus{
		InProgress: true,
		Phase:      "clearing_db",
		Message:    "Clearing database records...",
	})
	if err := a.database.DB.Exec("DELETE FROM assets").Error; err != nil {
		wailsRuntime.EventsEmit(a.ctx, "clear:error", ClearDataStatus{
			InProgress: false,
			Phase:      "error",
			Error:      err.Error(),
		})
		return fmt.Errorf("failed to delete assets: %w", err)
	}
	if err := a.database.DB.Exec("DELETE FROM nfts").Error; err != nil {
		wailsRuntime.EventsEmit(a.ctx, "clear:error", ClearDataStatus{
			InProgress: false,
			Phase:      "error",
			Error:      err.Error(),
		})
		return fmt.Errorf("failed to delete nfts: %w", err)
	}
	
	// Step 4: Update disk usage
	a.backupService.GetManager().MarkDiskUsageDirty()
	a.backupService.GetManager().UpdateDiskUsage()
	
	// Emit complete
	wailsRuntime.EventsEmit(a.ctx, "clear:complete", ClearDataStatus{
		InProgress:    false,
		Phase:         "complete",
		Message:       fmt.Sprintf("Cleared %d pins", unpinned),
		UnpinnedCount: unpinned,
	})
	
	log.Println("Full data reset complete")
	return nil
}

// RepinZeroSizeAssets re-pins all assets that are marked as pinned but have 0 or negative size
// These assets likely weren't actually pinned properly
func (a *App) RepinZeroSizeAssets() (int, error) {
	var assets []db.Asset
	if err := a.database.DB.Where("status = ? AND size_bytes <= 0", db.StatusPinned).Find(&assets).Error; err != nil {
		return 0, fmt.Errorf("failed to query assets: %w", err)
	}
	
	log.Printf("Found %d assets with zero/negative size to repin", len(assets))
	
	count := 0
	for _, asset := range assets {
		// Reset to pending so backup manager will re-process
		asset.Status = db.StatusPending
		asset.RetryCount = 0
		asset.PinnedAt = nil
		if err := a.database.SaveAsset(&asset); err != nil {
			log.Printf("Failed to reset asset %d: %v", asset.ID, err)
			continue
		}
		count++
	}
	
	log.Printf("Reset %d assets for re-pinning", count)
	return count, nil
}

// VerifyAndFixPins checks all pinned assets and updates their sizes from IPFS
func (a *App) VerifyAndFixPins() (map[string]int, error) {
	var assets []db.Asset
	if err := a.database.DB.Where("status = ?", db.StatusPinned).Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("failed to query assets: %w", err)
	}
	
	results := map[string]int{
		"total":   len(assets),
		"updated": 0,
		"failed":  0,
		"already_valid": 0,
	}
	
	log.Printf("Verifying %d pinned assets", len(assets))
	
	for _, asset := range assets {
		// Extract CID
		cid := extractCIDFromURI(asset.URI)
		if cid == "" {
			results["failed"]++
			continue
		}
		
		// Try to get size from IPFS
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		size, err := a.ipfsNode.Stat(ctx, cid)
		cancel()
		
		if err != nil {
			// Content not actually pinned/available
			log.Printf("Asset %s not available, marking for repin: %v", cid, err)
			asset.Status = db.StatusPending
			asset.RetryCount = 0
			a.database.SaveAsset(&asset)
			results["failed"]++
			continue
		}
		
		if asset.SizeBytes != size {
			asset.SizeBytes = size
			a.database.SaveAsset(&asset)
			results["updated"]++
			log.Printf("Updated size for %s: %d bytes", cid, size)
		} else {
			results["already_valid"]++
		}
	}
	
	log.Printf("Verify complete: %d updated, %d failed, %d already valid", 
		results["updated"], results["failed"], results["already_valid"])
	return results, nil
}

// VerifyAsset verifies a single asset is pinned and retrievable
func (a *App) VerifyAsset(assetID uint64) (ipfs.VerifyResult, error) {
	var asset db.Asset
	if err := a.database.DB.First(&asset, assetID).Error; err != nil {
		return ipfs.VerifyResult{Error: "asset not found"}, err
	}

	cid := extractCIDFromURI(asset.URI)
	if cid == "" {
		return ipfs.VerifyResult{Error: "could not extract CID"}, fmt.Errorf("could not extract CID from URI")
	}

	result := a.ipfsNode.Verify(a.ctx, cid, 30*time.Second)
	return result, nil
}

// PreviewAsset returns a preview of an asset's content (first N bytes)
func (a *App) PreviewAsset(assetID uint64, maxBytes int) (map[string]interface{}, error) {
	var asset db.Asset
	if err := a.database.DB.First(&asset, assetID).Error; err != nil {
		return nil, fmt.Errorf("asset not found: %w", err)
	}

	cid := extractCIDFromURI(asset.URI)
	if cid == "" {
		return nil, fmt.Errorf("could not extract CID from URI")
	}

	if maxBytes <= 0 {
		maxBytes = 1024 * 100 // 100KB default
	}

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	data, mimeType, err := a.ipfsNode.Cat(ctx, cid, int64(maxBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to get content: %w", err)
	}

	// For images, encode as base64 data URI
	result := map[string]interface{}{
		"cid":       cid,
		"mime_type": mimeType,
		"size":      len(data),
		"truncated": len(data) == maxBytes,
	}

	// For images, include base64 data
	if strings.HasPrefix(mimeType, "image/") {
		result["data_uri"] = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
	} else if mimeType == "application/json" || strings.HasPrefix(mimeType, "text/") {
		result["text"] = string(data)
	}

	return result, nil
}

// GetAssetGatewayURL returns public gateway URLs for an asset
func (a *App) GetAssetGatewayURL(assetID uint64) (map[string]string, error) {
	var asset db.Asset
	if err := a.database.DB.First(&asset, assetID).Error; err != nil {
		return nil, fmt.Errorf("asset not found: %w", err)
	}

	cid := extractCIDFromURI(asset.URI)
	if cid == "" {
		return nil, fmt.Errorf("could not extract CID from URI")
	}

	return map[string]string{
		"ipfs_io":      fmt.Sprintf("https://ipfs.io/ipfs/%s", cid),
		"dweb":         fmt.Sprintf("https://dweb.link/ipfs/%s", cid),
		"cloudflare":   fmt.Sprintf("https://cloudflare-ipfs.com/ipfs/%s", cid),
		"pinata":       fmt.Sprintf("https://gateway.pinata.cloud/ipfs/%s", cid),
		"local":        fmt.Sprintf("http://127.0.0.1:8080/ipfs/%s", cid),
	}, nil
}

// extractCIDFromURI extracts a CID from an IPFS URI
func extractCIDFromURI(uri string) string {
	// Common patterns:
	// ipfs://QmXXX
	// https://ipfs.io/ipfs/QmXXX

	if len(uri) > 7 && uri[:7] == "ipfs://" {
		return strings.Split(uri[7:], "/")[0]
	}

	// Find /ipfs/ in the URI
	const ipfsPrefix = "/ipfs/"
	idx := strings.Index(uri, ipfsPrefix)
	if idx != -1 {
		start := idx + len(ipfsPrefix)
		rest := uri[start:]
		// Find end (next / or end of string)
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			return rest[:slashIdx]
		}
		return rest
	}

	return ""
}

// ==================== Storage Management ====================

// GetStorageLocation returns information about the current storage location
func (a *App) GetStorageLocation() (*storage.StorageLocation, error) {
	repoPath := a.ipfsNode.GetRepoPath()
	return storage.GetStorageInfo(repoPath)
}

// ListStorageLocations returns all available storage locations
func (a *App) ListStorageLocations() ([]*storage.StorageLocation, error) {
	return storage.ListAvailableLocations()
}

// ValidateStoragePath checks if a path is valid for storage
func (a *App) ValidateStoragePath(path string) error {
	return storage.ValidatePath(path)
}

// GetStorageType detects what type of storage a path is
func (a *App) GetStorageType(path string) (string, error) {
	storageType, err := storage.DetectStorageType(path)
	if err != nil {
		return "", err
	}
	return string(storageType), nil
}

// MigrateStorage moves the IPFS repository to a new location
// This will stop the backup service, move the data, and restart with new location
func (a *App) MigrateStorage(destPath string) error {
	log.Printf("MigrateStorage called with destination: %s", destPath)
	
	// Validate destination first
	log.Println("Validating destination path...")
	if err := storage.ValidatePath(destPath); err != nil {
		log.Printf("Validation failed: %v", err)
		return fmt.Errorf("invalid destination: %w", err)
	}
	log.Println("Destination validated successfully")

	// Get current path
	currentPath := a.ipfsNode.GetRepoPath()
	log.Printf("Current IPFS path: %s", currentPath)
	
	// Check if same path
	expandedDest, _ := storage.ExpandPath(destPath)
	if currentPath == expandedDest {
		return fmt.Errorf("destination is same as current location")
	}

	// Emit starting event
	wailsRuntime.EventsEmit(a.ctx, "storage:migration:start", map[string]interface{}{
		"source": currentPath,
		"dest":   destPath,
	})

	// Stop backup service
	log.Println("Stopping backup service for migration...")
	a.backupService.Stop()

	// Stop IPFS node
	log.Println("Stopping IPFS node for migration...")
	if err := a.ipfsNode.Stop(); err != nil {
		a.backupService.Start(a.ctx) // Try to restart
		return fmt.Errorf("failed to stop IPFS node: %w", err)
	}
	log.Println("IPFS node stopped, starting migration...")

	// Create storage manager and perform migration
	manager := storage.NewManager(currentPath)
	
	err := manager.Migrate(a.ctx, destPath, func(status storage.MigrationStatus) {
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:progress", status)
	})

	if err != nil {
		// Try to restart with old path
		log.Printf("Migration failed, attempting to restart with old path: %v", err)
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:error", err.Error())
		
		newNode, nodeErr := ipfs.NewNode(currentPath, a.config.IPFS.SwarmPort)
		if nodeErr == nil {
			nodeErr = newNode.Start(a.ctx)
			if nodeErr == nil {
				a.ipfsNode = newNode
			}
		}
		a.backupService.Start(a.ctx)
		return fmt.Errorf("migration failed: %w", err)
	}

	// Update config with new path
	a.config.IPFS.RepoPath = expandedDest
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".porcupin", "config.yaml")
	if err := a.config.SaveConfig(configPath); err != nil {
		log.Printf("Warning: failed to save config: %v", err)
	}

	// Start IPFS node with new path
	log.Printf("Starting IPFS node at new location: %s", expandedDest)
	newNode, err := ipfs.NewNode(expandedDest, a.config.IPFS.SwarmPort)
	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:error", err.Error())
		return fmt.Errorf("failed to create node at new location: %w", err)
	}

	if err := newNode.Start(a.ctx); err != nil {
		wailsRuntime.EventsEmit(a.ctx, "storage:migration:error", err.Error())
		return fmt.Errorf("failed to start node at new location: %w", err)
	}

	a.ipfsNode = newNode

	// Restart backup service
	a.backupService.Start(a.ctx)
	
	// Update disk usage for new location
	a.backupService.GetManager().MarkDiskUsageDirty()
	a.backupService.GetManager().UpdateDiskUsage()

	wailsRuntime.EventsEmit(a.ctx, "storage:migration:complete", map[string]interface{}{
		"new_path": expandedDest,
	})

	log.Printf("Storage migration complete: %s -> %s", currentPath, expandedDest)
	return nil
}

// GetMigrationStatus returns the current migration status
func (a *App) GetMigrationStatus() storage.MigrationStatus {
	return storage.GetGlobalMigrationStatus()
}

// CancelMigration cancels an ongoing storage migration
func (a *App) CancelMigration() error {
	log.Println("CancelMigration called")
	err := storage.CancelGlobalMigration()
	if err != nil {
		log.Printf("CancelMigration error: %v", err)
		return err
	}
	
	wailsRuntime.EventsEmit(a.ctx, "storage:migration:cancelled", nil)
	
	// Restart IPFS and backup service with original path
	log.Println("Restarting services after cancellation...")
	currentPath := a.ipfsNode.GetRepoPath()
	
	newNode, err := ipfs.NewNode(currentPath, a.config.IPFS.SwarmPort)
	if err != nil {
		log.Printf("Failed to create node after cancel: %v", err)
		return fmt.Errorf("failed to restart IPFS: %w", err)
	}
	
	if err := newNode.Start(a.ctx); err != nil {
		log.Printf("Failed to start node after cancel: %v", err)
		return fmt.Errorf("failed to restart IPFS: %w", err)
	}
	
	a.ipfsNode = newNode
	a.backupService.Start(a.ctx)
	
	log.Println("Services restarted after migration cancellation")
	return nil
}

// BrowseForFolder opens a folder picker dialog
func (a *App) BrowseForFolder() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Storage Location",
	})
}
