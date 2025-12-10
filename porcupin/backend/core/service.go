package core

import (
	"context"
	"log"
	"sync"
	"time"

	"porcupin/backend/config"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
)

// ServiceState represents the current state of the backup service
type ServiceState string

const (
	StateStarting  ServiceState = "starting"
	StateSyncing   ServiceState = "syncing"
	StateWatching  ServiceState = "watching"
	StatePaused    ServiceState = "paused"
	StateStopped   ServiceState = "stopped"
)

// ServiceStatus represents the current status of the backup service
type ServiceStatus struct {
	State           ServiceState `json:"state"`
	Message         string       `json:"message"`
	IsPaused        bool         `json:"is_paused"`
	CurrentWallet   string       `json:"current_wallet"`
	WalletsTotal    int          `json:"wallets_total"`
	WalletsSynced   int          `json:"wallets_synced"`
	TotalNFTs       int          `json:"total_nfts"`
	ProcessedNFTs   int          `json:"processed_nfts"`
	TotalAssets     int          `json:"total_assets"`
	PinnedAssets    int          `json:"pinned_assets"`
	FailedAssets    int          `json:"failed_assets"`
	PendingRetries  int          `json:"pending_retries"`
	CurrentItem     string       `json:"current_item"`
	LastSyncAt      *time.Time   `json:"last_sync_at"`
}

// BackupService manages the automatic backup lifecycle
type BackupService struct {
	manager  *BackupManager
	indexer  *indexer.Indexer
	db       *db.Database
	config   *config.Config
	ipfs     *ipfs.Node
	
	ctx       context.Context
	cancel    context.CancelFunc
	
	mu        sync.RWMutex
	status    ServiceStatus
	isPaused  bool
	
	// Channels for coordination
	pauseCh   chan struct{}
	resumeCh  chan struct{}
	triggerCh chan string  // wallet address to sync
}

// NewBackupService creates a new backup service
func NewBackupService(ipfsNode *ipfs.Node, idx *indexer.Indexer, database *db.Database, cfg *config.Config) *BackupService {
	manager := NewBackupManager(ipfsNode, idx, database, cfg)
	
	return &BackupService{
		manager:   manager,
		indexer:   idx,
		db:        database,
		config:    cfg,
		ipfs:      ipfsNode,
		status:    ServiceStatus{State: StateStopped},
		pauseCh:   make(chan struct{}),
		resumeCh:  make(chan struct{}),
		triggerCh: make(chan string, 100),
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
	go s.run()
	
	// Start the retry worker
	go s.retryWorker()
	
	log.Println("Backup service started")
}

// run is the main service loop
func (s *BackupService) run() {
	// Phase 1: Initial catch-up sync for all wallets
	s.performCatchUpSync()
	
	// Phase 2: Start WebSocket listeners for real-time updates
	s.startWatching()
	
	// Phase 3: Periodic health check
	healthTicker := time.NewTicker(5 * time.Minute)
	defer healthTicker.Stop()
	
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
			// Wait for resume
			select {
			case <-s.resumeCh:
				s.updateStatus(func(st *ServiceStatus) {
					st.State = StateWatching
					st.IsPaused = false
					st.Message = "Watching for new NFTs"
				})
			case <-s.ctx.Done():
				return
			}
			
		case walletAddr := <-s.triggerCh:
			// Manual or WebSocket triggered sync for a specific wallet
			if !s.isPaused {
				s.syncWallet(walletAddr)
			}
			
		case <-healthTicker.C:
			// Periodic check - sync any wallets that haven't been synced in a while
			if !s.isPaused {
				s.performHealthCheck()
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
		log.Printf("Failed to get wallets for catch-up sync: %v", err)
		return
	}
	
	s.updateStatus(func(st *ServiceStatus) {
		st.WalletsTotal = len(wallets)
		st.WalletsSynced = 0
	})
	
	for i, wallet := range wallets {
		if s.isPaused {
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
			log.Printf("Failed to sync wallet %s: %v", wallet.Address, err)
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
		log.Printf("Failed to get wallets for watching: %v", err)
		return
	}
	
	for _, wallet := range wallets {
		go s.watchWallet(wallet.Address)
	}
}

// watchWallet sets up WebSocket watching for a single wallet
func (s *BackupService) watchWallet(address string) {
	s.watchWalletWithRetry(address, 0)
}

// watchWalletWithRetry is the internal implementation with crash counter
func (s *BackupService) watchWalletWithRetry(address string, crashCount int) {
	// Give up after too many crashes - rely on health check polling instead
	if crashCount >= 5 {
		log.Printf("WebSocket watcher for %s crashed too many times (%d), disabling. Will use polling.", address, crashCount)
		return
	}

	// Recover from panics in the WebSocket library
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WebSocket watcher for %s crashed (%d): %v, will restart in 60s", address, crashCount+1, r)
			time.Sleep(60 * time.Second)
			// Restart the watcher with incremented crash count
			go s.watchWalletWithRetry(address, crashCount+1)
		}
	}()

	// Create a dedicated indexer for this wallet's WebSocket connection
	idx := indexer.NewIndexer(s.config.TZKT.BaseURL)
	
	// Set up the callback for when new tokens are received
	idx.SetTokenCallback(func(token indexer.Token) {
		// Don't trigger syncs when paused
		if s.IsPaused() {
			log.Printf("WebSocket: Ignoring token update for %s (paused)", address)
			return
		}
		
		log.Printf("WebSocket: New token received for %s: %s", address, token.TokenID)
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
		case <-s.ctx.Done():
			idx.Close()
			return
		default:
		}
		
		// Listen blocks until connection closes or context cancelled
		if err := idx.Listen(s.ctx, address); err != nil {
			log.Printf("WebSocket connection failed for %s: %v, reconnecting in 30s", address, err)
			time.Sleep(30 * time.Second)
			continue
		}
		
		// Connection closed normally, wait before reconnecting
		log.Printf("WebSocket connection closed for %s, reconnecting in 30s", address)
		time.Sleep(30 * time.Second)
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
		log.Printf("Failed to sync wallet %s: %v", address, err)
	} else if headLevel > 0 {
		s.db.UpdateWalletSyncTime(address, headLevel)
	}
	
	s.updateStatus(func(st *ServiceStatus) {
		st.State = StateWatching
		st.CurrentWallet = ""
		st.Message = "Watching for new NFTs"
	})
}

// retryWorker periodically retries failed and pending assets
func (s *BackupService) retryWorker() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.isPaused {
				continue
			}
			s.retryFailedAssets()
			s.processPendingAssets()
		}
	}
}

// processPendingAssets processes assets stuck in pending status
func (s *BackupService) processPendingAssets() {
	processed, pinned, failed := s.manager.ProcessPendingAssets(s.ctx, 50)
	if processed > 0 {
		log.Printf("Processed %d pending assets: %d pinned, %d failed", processed, pinned, failed)
	}
}

// retryFailedAssets retries assets that have failed
func (s *BackupService) retryFailedAssets() {
	assets, err := s.db.GetRetryableAssets(5, 50) // max 5 retries, 50 at a time
	if err != nil {
		log.Printf("Failed to get retryable assets: %v", err)
		return
	}
	
	if len(assets) == 0 {
		return
	}
	
	s.updateStatus(func(st *ServiceStatus) {
		st.PendingRetries = len(assets)
	})
	
	log.Printf("Retrying %d failed assets", len(assets))
	
	for _, asset := range assets {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		
		if s.isPaused {
			return
		}
		
		// Reset status to pending
		asset.Status = db.StatusPending
		s.db.SaveAsset(&asset)
		
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
			log.Printf("Health check: Wallet %s needs sync (last: %v)", wallet.Address, wallet.LastSyncedAt)
			select {
			case s.triggerCh <- wallet.Address:
			default:
			}
		}
	}
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

// TriggerFullSync triggers a full sync for all wallets
func (s *BackupService) TriggerFullSync() {
	go s.performCatchUpSync()
}

// Stop stops the backup service
func (s *BackupService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.manager.Shutdown()
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
	
	// Start watching
	go s.watchWallet(address)
}

// PinAsset triggers immediate pinning of a specific asset
func (s *BackupService) PinAsset(ctx context.Context, assetID uint64) error {
	return s.manager.PinAssetByID(ctx, assetID)
}

// UnpinAsset unpins an asset by CID
func (s *BackupService) UnpinAsset(cid string) error {
	if s.ipfs == nil {
		return nil
	}
	return s.ipfs.Unpin(s.ctx, cid)
}
