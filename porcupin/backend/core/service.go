package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"porcupin/backend/config"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
	"porcupin/backend/storage"

	"gorm.io/gorm"
)

// ServiceState represents the current state of the backup service
type ServiceState string

const (
	StateStarting ServiceState = "starting"
	StateSyncing  ServiceState = "syncing"
	StateWatching ServiceState = "watching"
	StatePaused   ServiceState = "paused"
	StateStopped  ServiceState = "stopped"
)

// ServiceStatus represents the current status of the backup service
type ServiceStatus struct {
	State          ServiceState `json:"state"`
	Message        string       `json:"message"`
	IsPaused       bool         `json:"is_paused"`
	CurrentWallet  string       `json:"current_wallet"`
	WalletsTotal   int          `json:"wallets_total"`
	WalletsSynced  int          `json:"wallets_synced"`
	TotalNFTs      int          `json:"total_nfts"`
	ProcessedNFTs  int          `json:"processed_nfts"`
	TotalAssets    int          `json:"total_assets"`
	PinnedAssets   int          `json:"pinned_assets"`
	FailedAssets   int          `json:"failed_assets"`
	PendingRetries int          `json:"pending_retries"`
	CurrentItem    string       `json:"current_item"`
	LastSyncAt     *time.Time   `json:"last_sync_at"`
}

// BackupService manages the automatic backup lifecycle
type BackupService struct {
	manager *BackupManager
	indexer *indexer.Indexer
	db      *db.Database
	config  *config.Config
	ipfs    *ipfs.Node
	storage *storage.Manager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.RWMutex
	status   ServiceStatus
	isPaused bool

	// Channels for coordination
	pauseCh    chan struct{}
	resumeCh   chan struct{}
	triggerCh  chan string   // wallet address to sync
	fullSyncCh chan struct{} // signal run() to perform a full catch-up sync

	watchedWallets map[string]bool // Track which wallets have active watchers

	// Wails context for event emission (set after wails.Run starts)
	wailsCtx       context.Context
	lastIPFSOnline bool

	clearing atomic.Bool // guards against concurrent ClearAllData calls
}

// NewBackupService creates a new backup service
func NewBackupService(ipfsNode *ipfs.Node, idx *indexer.Indexer, database *db.Database, cfg *config.Config) *BackupService {
	manager := NewBackupManager(ipfsNode, idx, database, cfg)

	// Create storage manager pointing to current repo path
	storageMgr := storage.NewManager(ipfsNode.GetRepoPath())

	return &BackupService{
		manager:        manager,
		indexer:        idx,
		db:             database,
		config:         cfg,
		ipfs:           ipfsNode,
		storage:        storageMgr,
		status:         ServiceStatus{State: StateStopped},
		pauseCh:        make(chan struct{}),
		resumeCh:       make(chan struct{}),
		triggerCh:      make(chan string, 100),
		fullSyncCh:     make(chan struct{}, 1), // buffer of 1 coalesces rapid triggers
		watchedWallets: make(map[string]bool),
	}
}

// Start begins the automatic backup service
func (s *BackupService) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateStarting
		st.Message = "Initializing backup service..."
	})

	// Start the main service loop
	s.wg.Add(3)
	go func() {
		defer s.wg.Done()
		s.run()
	}()

	// Start the retry worker
	go func() {
		defer s.wg.Done()
		s.retryWorker()
	}()

	slog.Info("backup service started")
}

// run is the main service loop
func (s *BackupService) run() {
	// Phase 1: Initial catch-up sync for all wallets
	s.performCatchUpSync()

	// Phase 2: Start WebSocket listeners for real-time updates
	s.startWatching()

	// Phase 2.5: Run integrity check in background to fix any missing asset records (e.g. from previous bugs)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered", "goroutine", "integrity-check", "panic", r)
			}
		}()
		slog.Info("running background integrity check")
		stats, err := s.manager.VerifyAndFixPins(s.ctx)
		if err != nil {
			slog.Error("background integrity check failed", "error", err)
		} else {
			slog.Info("background integrity check complete", "checked", stats["checked"], "processed", stats["processed"], "errors", stats["errors"])
		}
	}()

	// Phase 3: Periodic health check
	healthTicker := time.NewTicker(5 * time.Minute)
	defer healthTicker.Stop()

	// Hot Reload Ticker: Check for new wallets frequently (every 15 seconds)
	hotReloadTicker := time.NewTicker(15 * time.Second)
	defer hotReloadTicker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.updateStatus(func(st *ServiceStatus) {
				st.State = StateStopped
				st.Message = "Service stopped"
			})
			return

		case <-s.pauseCh:
			s.updateStatus(func(st *ServiceStatus) {
				st.State = StatePaused
				st.IsPaused = true
				st.Message = "Backup paused"
			})
			// Wait for resume but keep updating disk usage
		PauseLoop:
			for {
				select {
				case <-s.resumeCh:
					s.updateStatus(func(st *ServiceStatus) {
						st.State = StateWatching
						st.IsPaused = false
						st.Message = "Watching for new NFTs"
					})
					break PauseLoop
				case <-time.After(1 * time.Minute):
					// Update disk usage even when paused
					s.manager.UpdateDiskUsage()
				case <-s.ctx.Done():
					return
				}
			}

		case walletAddr := <-s.triggerCh:
			// Manual or WebSocket triggered sync for a specific wallet
			if !s.IsPaused() {
				s.syncWallet(walletAddr)
			}

		case <-hotReloadTicker.C:
			// fast check for new wallets added via CLI
			if !s.IsPaused() {
				s.checkForNewWallets()
			}

		case <-healthTicker.C:
			// Periodic check - sync any wallets that haven't been synced in a while
			if !s.IsPaused() {
				s.performHealthCheck()
			}

		case <-s.fullSyncCh:
			// User-triggered full sync — runs synchronously inside run() so it is
			// covered by the WaitGroup and shutdown waits for it to complete.
			if !s.IsPaused() {
				s.performCatchUpSync()
			}
		}
	}
}

// performCatchUpSync syncs all wallets that need catching up
func (s *BackupService) performCatchUpSync() {
	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateSyncing
		st.Message = "Catching up on missed NFTs..."
	})

	wallets, err := s.db.GetAllWallets()
	if err != nil {
		slog.Error("failed to get wallets for catch-up sync", "error", err)
		return
	}

	s.updateStatus(func(st *ServiceStatus) {
		st.WalletsTotal = len(wallets)
		st.WalletsSynced = 0
	})

	for i, wallet := range wallets {
		if s.IsPaused() {
			break
		}

		select {
		case <-s.ctx.Done():
			return
		default:
		}

		s.updateStatus(func(st *ServiceStatus) {
			st.CurrentWallet = wallet.Address
			st.Message = "Syncing wallet " + wallet.Address[:8] + "..."
		})

		headLevel, err := s.manager.SyncWallet(s.ctx, wallet.Address)
		if err != nil {
			slog.Error("failed to sync wallet", "wallet", wallet.Address, "error", err)
		} else if headLevel > 0 {
			// Update wallet sync time with the head level we synced up to
			s.db.UpdateWalletSyncTime(wallet.Address, headLevel)
		}

		s.updateStatus(func(st *ServiceStatus) {
			st.WalletsSynced = i + 1
		})
	}

	now := time.Now()
	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateWatching
		st.Message = "Sync complete, watching for new NFTs"
		st.LastSyncAt = &now
		st.CurrentWallet = ""
		// Clear sync progress counters
		st.TotalNFTs = 0
		st.ProcessedNFTs = 0
		st.TotalAssets = 0
		st.PinnedAssets = 0
		st.FailedAssets = 0
		st.CurrentItem = ""
	})
}

// startWatching starts WebSocket listeners for all wallets
func (s *BackupService) startWatching() {
	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateWatching
		st.Message = "Watching for new NFTs"
	})

	wallets, err := s.db.GetAllWallets()
	if err != nil {
		slog.Error("failed to get wallets for watching", "error", err)
		return
	}

	// Lock for map access
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, wallet := range wallets {
		if !s.watchedWallets[wallet.Address] {
			s.watchedWallets[wallet.Address] = true
			go s.watchWallet(wallet.Address)
		}
	}
}

// watchWallet sets up WebSocket watching for a single wallet.
// Captures s.ctx at call time so this watcher is tied to the current
// service lifecycle and is not affected by future Stop/Start cycles.
func (s *BackupService) watchWallet(address string) {
	s.watchWalletWithRetry(address, 0, s.ctx)
}

// watchWalletWithRetry is the internal implementation with crash counter.
// ctx is the service context captured when this watcher was launched.
func (s *BackupService) watchWalletWithRetry(address string, crashCount int, ctx context.Context) {
	// Give up after too many crashes - rely on health check polling instead
	if crashCount >= 5 {
		slog.Error("websocket watcher crashed too many times, disabling", "wallet", address, "crash_count", crashCount)
		return
	}

	// Recover from panics in the WebSocket library
	defer func() {
		if r := recover(); r != nil {
			// Don't restart if the service has been stopped
			if ctx.Err() != nil {
				return
			}
			slog.Error("websocket watcher crashed, will restart", "wallet", address, "crash_count", crashCount+1, "panic", r, "restart_delay", "60s")
			// Use a ctx-aware wait so shutdown is not delayed by up to 60s
			select {
			case <-time.After(60 * time.Second):
			case <-ctx.Done():
				return
			}
			go s.watchWalletWithRetry(address, crashCount+1, ctx)
		}
	}()

	// Create a dedicated indexer for this wallet's WebSocket connection
	idx := indexer.NewIndexer(s.config.TZKT.BaseURL)

	// Set up the callback for when new tokens are received
	idx.SetTokenCallback(func(token indexer.Token) {
		// Don't trigger syncs when paused
		if s.IsPaused() {
			slog.Debug("ignoring token update, service paused", "wallet", address)
			return
		}

		slog.Info("new token received via websocket", "wallet", address, "token_id", token.TokenID)
		// Trigger a sync for this wallet
		select {
		case s.triggerCh <- address:
		default:
			// Channel full, will catch up on next health check
		}
	})

	for {
		// Check context before attempting connection
		select {
		case <-ctx.Done():
			idx.Close()
			return
		default:
		}

		// Listen blocks until connection closes or context cancelled
		if err := idx.Listen(ctx, address); err != nil {
			slog.Warn("websocket connection failed, reconnecting", "wallet", address, "error", err, "retry_delay", "30s")
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				idx.Close()
				return
			}
			continue
		}

		// Connection closed normally, wait before reconnecting
		slog.Info("websocket connection closed, reconnecting", "wallet", address, "retry_delay", "30s")
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			idx.Close()
			return
		}
	}
}

// syncWallet syncs a single wallet
func (s *BackupService) syncWallet(address string) {
	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateSyncing
		st.CurrentWallet = address
		st.Message = "Syncing " + address[:8] + "..."
	})

	headLevel, err := s.manager.SyncWallet(s.ctx, address)
	if err != nil {
		slog.Error("failed to sync wallet", "wallet", address, "error", err)
	} else if headLevel > 0 {
		s.db.UpdateWalletSyncTime(address, headLevel)
	}

	now := time.Now()
	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateWatching
		st.CurrentWallet = ""
		st.Message = "Watching for new NFTs"
		st.LastSyncAt = &now
	})
}

// retryWorker periodically retries failed and pending assets
func (s *BackupService) retryWorker() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("goroutine panic recovered", "goroutine", "retry-worker", "panic", r)
		}
	}()
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.IsPaused() {
				continue
			}
			s.retryFailedAssets()
			s.processPendingAssets()
		}
	}
}

// processPendingAssets processes assets stuck in pending status
func (s *BackupService) processPendingAssets() {
	// Update status so user sees activity
	s.updateStatus(func(st *ServiceStatus) {
		st.Message = "Processing pending assets..."
	})

	processed, pinned, failed := s.manager.ProcessPendingAssets(s.ctx, 50)
	if processed > 0 {
		slog.Info("processed pending assets", "processed", processed, "pinned", pinned, "failed", failed)
	}

	// Restore status
	s.updateStatus(func(st *ServiceStatus) {
		st.Message = "Watching for new NFTs"
	})
}

// retryFailedAssets retries assets that have failed
func (s *BackupService) retryFailedAssets() {
	assets, err := s.db.GetRetryableAssets(5, 50) // max 5 retries, 50 at a time
	if err != nil {
		slog.Error("failed to get retryable assets", "error", err)
		return
	}

	if len(assets) == 0 {
		return
	}

	s.updateStatus(func(st *ServiceStatus) {
		st.PendingRetries = len(assets)
		st.Message = fmt.Sprintf("Retrying %d failed assets...", len(assets))
	})

	slog.Info("retrying failed assets", "count", len(assets))

	for _, asset := range assets {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		if s.IsPaused() {
			return
		}

		// Reset status to pending
		asset.Status = db.StatusPending
		if err := s.db.SaveAsset(&asset); err != nil {
			slog.Warn("failed to reset asset to pending", "asset_id", asset.ID, "error", err)
		}

		// The BackupManager's processNFT will pick this up
		// For now, we just mark them as pending and let the next sync handle them
	}
}

// performHealthCheck checks for any wallets that need syncing
func (s *BackupService) performHealthCheck() {
	// Update disk usage if any pins happened
	s.manager.UpdateDiskUsage()

	wallets, err := s.db.GetAllWallets()
	if err != nil {
		return
	}

	staleThreshold := time.Now().Add(-1 * time.Hour)

	for _, wallet := range wallets {
		if wallet.LastSyncedAt == nil || wallet.LastSyncedAt.Before(staleThreshold) {
			slog.Debug("health check: wallet needs sync", "wallet", wallet.Address, "last_synced_at", wallet.LastSyncedAt)
			select {
			case s.triggerCh <- wallet.Address:
			default:
			}
		}
	}

	// IPFS connectivity check — emit event only on state change
	s.mu.RLock()
	ipfsNode := s.ipfs
	s.mu.RUnlock()
	if ipfsNode != nil {
		health := ipfsNode.Health(s.ctx)
		if health.IsOnline != s.lastIPFSOnline {
			s.lastIPFSOnline = health.IsOnline
			s.mu.RLock()
			wCtx := s.wailsCtx
			s.mu.RUnlock()
			if wCtx != nil {
				wailsRuntime.EventsEmit(wCtx, "ipfs:health", health)
			}
		}
	}
}

// SetWailsCtx provides the Wails context needed to emit events to the frontend.
// Call this from app.go after wails.Run() sets up the runtime.
func (s *BackupService) SetWailsCtx(ctx context.Context) {
	s.mu.Lock()
	s.wailsCtx = ctx
	s.mu.Unlock()
}

// Pause pauses the backup service
func (s *BackupService) Pause() {
	s.mu.Lock()
	s.isPaused = true
	s.mu.Unlock()

	// Also pause the backup manager to stop in-progress work
	s.manager.SetPaused(true)

	select {
	case s.pauseCh <- struct{}{}:
	default:
	}
}

// Resume resumes the backup service
func (s *BackupService) Resume() {
	s.mu.Lock()
	s.isPaused = false
	s.mu.Unlock()

	// Resume the backup manager
	s.manager.SetPaused(false)

	select {
	case s.resumeCh <- struct{}{}:
	default:
	}
}

// IsPaused returns whether the service is paused
func (s *BackupService) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isPaused
}

// GetStatus returns the current service status
func (s *BackupService) GetStatus() ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Merge with BackupManager progress
	progress := s.manager.GetProgress()
	status := s.status

	if progress.IsActive {
		status.TotalNFTs = progress.TotalNFTs
		status.ProcessedNFTs = progress.ProcessedNFTs
		status.TotalAssets = progress.TotalAssets
		status.PinnedAssets = progress.PinnedAssets
		status.FailedAssets = progress.FailedAssets
		status.CurrentItem = progress.CurrentItem
		if progress.Message != "" {
			status.Message = progress.Message
		}
	}

	return status
}

// TriggerSync manually triggers a sync for a wallet
func (s *BackupService) TriggerSync(address string) {
	select {
	case s.triggerCh <- address:
	default:
	}
}

// TriggerFullSync signals the run() loop to perform a full catch-up sync.
// Uses a buffered channel (cap 1) so multiple rapid calls coalesce into one sync.
func (s *BackupService) TriggerFullSync() {
	select {
	case s.fullSyncCh <- struct{}{}:
	default: // a sync is already queued; do nothing
	}
}

// Stop stops the backup service and waits for goroutines to exit.
func (s *BackupService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.manager.Shutdown()

	// Reset watcher tracking so the next Start() re-launches watchers.
	// Old watchers exit when their captured ctx is cancelled above.
	s.mu.Lock()
	s.watchedWallets = make(map[string]bool)
	s.mu.Unlock()
}

// UpdateIPFS replaces the IPFS node and storage manager refs (used after storage migration).
func (s *BackupService) UpdateIPFS(node *ipfs.Node) {
	s.mu.Lock()
	s.ipfs = node
	s.storage = storage.NewManager(node.GetRepoPath())
	s.mu.Unlock()
	s.manager.UpdateIPFS(node)
}

// GetManager returns the underlying backup manager
func (s *BackupService) GetManager() *BackupManager {
	return s.manager
}

// updateStatus safely updates the service status
func (s *BackupService) updateStatus(fn func(*ServiceStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.status)
}

// AddWallet adds a new wallet and starts watching it
func (s *BackupService) AddWallet(address string) {
	// Trigger immediate sync
	select {
	case s.triggerCh <- address:
	default:
	}

	// Start watching only if not already tracked; prevents duplicate watchers
	// when checkForNewWallets runs its next 15s tick.
	s.mu.Lock()
	if !s.watchedWallets[address] {
		s.watchedWallets[address] = true
		s.mu.Unlock()
		go s.watchWallet(address)
	} else {
		s.mu.Unlock()
	}
}

// PinAsset triggers immediate pinning of a specific asset
func (s *BackupService) PinAsset(ctx context.Context, assetID uint64) error {
	return s.manager.PinAssetByID(ctx, assetID)
}

// UnpinAsset unpins an asset by CID
func (s *BackupService) UnpinAsset(cid string) error {
	s.mu.RLock()
	ipfsNode := s.ipfs
	s.mu.RUnlock()
	if ipfsNode == nil {
		return nil
	}
	return ipfsNode.Unpin(s.ctx, cid)
}

// VerifyAndFixPins runs the verification and repair process
func (s *BackupService) VerifyAndFixPins() (map[string]int, error) {
	return s.manager.VerifyAndFixPins(s.ctx)
}

// VerifyPinHealth checks all pinned assets for IPFS availability and corrects sizes.
// Unlike VerifyAndFixPins, this does not re-process NFTs or discover missing assets.
func (s *BackupService) VerifyPinHealth() (map[string]int, error) {
	s.mu.RLock()
	ipfsNode := s.ipfs
	database := s.db
	s.mu.RUnlock()

	if ipfsNode == nil {
		return nil, fmt.Errorf("IPFS node not available")
	}

	var assets []db.Asset
	if err := database.DB.Where("status = ?", db.StatusPinned).Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("failed to query assets: %w", err)
	}

	results := map[string]int{
		"total":         len(assets),
		"updated":       0,
		"failed":        0,
		"already_valid": 0,
		"db_errors":     0,
	}

	for _, asset := range assets {
		cid := ExtractCIDFromURI(asset.URI)
		if cid == "" {
			results["failed"]++
			continue
		}

		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		size, err := ipfsNode.Stat(ctx, cid)
		cancel()

		if err != nil {
			asset.Status = db.StatusPending
			asset.RetryCount = 0
			if dbErr := database.SaveAsset(&asset); dbErr != nil {
				slog.Warn("failed to save asset status", "uri", asset.URI, "error", dbErr)
				results["db_errors"]++
			}
			results["failed"]++
			continue
		}

		if asset.SizeBytes != size {
			asset.SizeBytes = size
			if dbErr := database.SaveAsset(&asset); dbErr != nil {
				slog.Warn("failed to save asset size", "uri", asset.URI, "error", dbErr)
				results["db_errors"]++
			} else {
				results["updated"]++
			}
		} else {
			results["already_valid"]++
		}
	}

	return results, nil
}

// GetStorageManager returns the storage manager instance
func (s *BackupService) GetStorageManager() *storage.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storage
}

// ClearAllData performs a full device reset: unpins everything, GCs, and clears DB
func (s *BackupService) ClearAllData(progress func(string, string, int, int)) error {
	if !s.clearing.CompareAndSwap(false, true) {
		return fmt.Errorf("clear operation already in progress")
	}
	defer s.clearing.Store(false)

	s.mu.RLock()
	ipfsNode := s.ipfs
	s.mu.RUnlock()

	// 1. Unpin all IPFS content
	if progress != nil {
		progress("unpinning", "Unpinning all IPFS content...", 0, 0)
	}

	unpinned, err := ipfsNode.UnpinAll(s.ctx, func(total, current int) {
		if progress != nil {
			progress("unpinning", fmt.Sprintf("Unpinning content... %d/%d", current, total), total, current)
		}
	})
	if err != nil {
		slog.Warn("failed to unpin all", "error", err)
	}
	slog.Info("unpinned items", "count", unpinned)

	// 2. Garbage Collect
	if progress != nil {
		progress("garbage_collect", "Running internal IPFS garbage collection...", 0, 0)
	}
	if err := ipfsNode.GarbageCollect(s.ctx); err != nil {
		slog.Warn("garbage collection failed", "error", err)
	}

	// 3. Clear DB
	if progress != nil {
		progress("clearing_db", "Clearing database records...", 0, 0)
	}

	// Transactional clear
	return s.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM assets").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM nfts").Error; err != nil {
			return err
		}
		// Reset wallets to sync=false/levels=0 instead of deleting?
		// Original implementation didn't delete wallets in ResetDatabase, it says "clearing all NFTs, assets".
		// Actually app.go ResetDatabase did DELETE FROM assets, nfts.
		// It did NOT delete wallets.
		return nil
	})
}

// DeleteWalletFull deletes a wallet and all its assets/NFTs from DB and unpins content
func (s *BackupService) DeleteWalletFull(address string, progress func(phase string, total, current int)) error {
	// Get all assets for this wallet
	assets, err := s.db.GetAssetsByWallet(address)
	if err != nil {
		return fmt.Errorf("failed to get assets: %w", err)
	}

	s.mu.RLock()
	ipfsNode := s.ipfs
	s.mu.RUnlock()

	// Count unpinnable assets for progress reporting
	total := 0
	for _, asset := range assets {
		if ExtractCIDFromURI(asset.URI) != "" {
			total++
		}
	}

	if progress != nil {
		progress("unpinning", total, 0)
	}

	// Unpin each asset — log errors but continue so DB cleanup always happens.
	unpinned := 0
	for _, asset := range assets {
		cid := ExtractCIDFromURI(asset.URI)
		if cid == "" {
			continue
		}
		if err := ipfsNode.Unpin(s.ctx, cid); err != nil {
			slog.Warn("failed to unpin asset", "cid", cid, "error", err)
		}
		unpinned++
		if progress != nil {
			progress("unpinning", total, unpinned)
		}
	}

	// Delete from database
	if progress != nil {
		progress("clearing_db", 0, 0)
	}
	if err := s.db.DeleteAssetsByWallet(address); err != nil {
		return fmt.Errorf("failed to delete assets: %w", err)
	}
	if err := s.db.DeleteNFTsByWallet(address); err != nil {
		return fmt.Errorf("failed to delete NFTs: %w", err)
	}
	if err := s.db.DeleteWallet(address); err != nil {
		return fmt.Errorf("failed to delete wallet: %w", err)
	}

	// Mark disk usage dirty so it recalculates
	s.manager.MarkDiskUsageDirty()

	return nil
}
