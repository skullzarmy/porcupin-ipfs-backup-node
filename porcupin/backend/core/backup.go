package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"porcupin/backend/config"
	"porcupin/backend/db"
	"porcupin/backend/httpx"
	"porcupin/backend/indexer"
	ipfsuri "porcupin/backend/uri"
)

// errAssetSkipped is returned by pinAssetDirect when a URI is not an IPFS URI.
// Callers use errors.Is to distinguish skipped assets from pin failures.
var errAssetSkipped = errors.New("asset skipped: not an IPFS URI")

// SyncProgress represents the current sync operation progress
type SyncProgress struct {
	IsActive      bool      `json:"is_active"`
	Phase         string    `json:"phase"` // "idle", "fetching", "processing", "pinning"
	WalletAddress string    `json:"wallet_address"`
	TotalNFTs     int       `json:"total_nfts"`
	ProcessedNFTs int       `json:"processed_nfts"`
	TotalAssets   int       `json:"total_assets"`
	PinnedAssets  int       `json:"pinned_assets"`
	FailedAssets  int       `json:"failed_assets"`
	CurrentItem   string    `json:"current_item"`
	StartedAt     time.Time `json:"started_at"`
	Message       string    `json:"message"`
}

// IPFSClient defines the subset of IPFS methods used by the backup manager
// This interface allows for mocking in tests
type IPFSClient interface {
	Pin(ctx context.Context, cid string, timeout time.Duration) error
	Unpin(ctx context.Context, cid string) error
	Stat(ctx context.Context, cid string) (int64, error)
	Cat(ctx context.Context, cid string, sizeLimit int64) ([]byte, string, error)
	GetRepoPath() string
}

// BackupManager orchestrates the backup process
type BackupManager struct {
	ipfs    IPFSClient
	indexer *indexer.Indexer
	db      *db.Database
	config  *config.Config
	mu      sync.RWMutex
	workers chan struct{}

	// heavyOpMu serializes large batch operations (SyncWallet,
	// VerifyAndFixPins, ProcessPendingAssets). Running them concurrently on
	// memory-constrained machines stacks goroutines + pin contexts onto the
	// same small worker pool and has been observed to trigger OOM kills.
	//
	// TRADE-OFF: a slow SyncWallet (TZKT fetches can take minutes on first
	// sync) holds this mutex for its full duration. While held:
	//   - VerifyAndFixPins (background integrity check) blocks until done.
	//   - ProcessPendingAssets (retry tick every 2 min) blocks until done.
	//   - Websocket-triggered syncs handled by service.run() block in the
	//     dispatch loop, delaying real-time updates for OTHER wallets until
	//     the in-flight sync completes.
	// This is the price of OOM-safety. If real-time latency becomes a
	// concern, restructure service.run() to handle triggers in their own
	// goroutines rather than synchronously.
	heavyOpMu sync.Mutex

	// Pause control
	pauseMu  sync.RWMutex
	isPaused bool

	// Sync progress tracking
	progressMu    sync.RWMutex
	progress      SyncProgress
	processedURIs atomic.Pointer[sync.Map] // tracks URIs processed in current sync to avoid double-counting

	// Disk usage tracking - update after pins, not on every pin
	diskUsageDirty int32 // atomic flag: 1 if pins happened since last du
}

// NewBackupManager creates a new backup manager
func NewBackupManager(ipfsNode IPFSClient, idx *indexer.Indexer, database *db.Database, cfg *config.Config) *BackupManager {
	// Safeguard: Ensure at least 1 worker, default to 5 if suspicious
	concurrency := cfg.Backup.MaxConcurrency
	if concurrency <= 0 {
		slog.Warn("MaxConcurrency invalid, defaulting to 5", "configured", concurrency)
		concurrency = 5
	}

	slog.Info("Initializing BackupManager", "workers", concurrency)

	bm := &BackupManager{
		ipfs:     ipfsNode,
		indexer:  idx,
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, concurrency),
		progress: SyncProgress{Phase: "idle"},
	}
	bm.processedURIs.Store(new(sync.Map))
	return bm
}

// UpdateIPFS replaces the IPFS node reference (used after storage migration).
func (bm *BackupManager) UpdateIPFS(node IPFSClient) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.ipfs = node
}

// getIPFS returns the current IPFS client under a read lock.
func (bm *BackupManager) getIPFS() IPFSClient {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.ipfs
}

// SetPaused sets the pause state
func (bm *BackupManager) SetPaused(paused bool) {
	bm.pauseMu.Lock()
	defer bm.pauseMu.Unlock()
	bm.isPaused = paused
	if paused {
		bm.updateProgress(func(p *SyncProgress) {
			p.Message = "Paused"
		})
	}
}

// IsPaused returns whether the manager is paused
func (bm *BackupManager) IsPaused() bool {
	bm.pauseMu.RLock()
	defer bm.pauseMu.RUnlock()
	return bm.isPaused
}

// GetProgress returns the current sync progress
func (bm *BackupManager) GetProgress() SyncProgress {
	bm.progressMu.RLock()
	defer bm.progressMu.RUnlock()
	return bm.progress
}

// updateProgress updates the sync progress
func (bm *BackupManager) updateProgress(fn func(*SyncProgress)) {
	bm.progressMu.Lock()
	defer bm.progressMu.Unlock()
	fn(&bm.progress)
}

// SyncWallet syncs all NFTs for a given wallet address
// Returns the blockchain level synced up to, or error
func (bm *BackupManager) SyncWallet(ctx context.Context, address string) (headLevel int64, err error) {
	// Serialize with VerifyAndFixPins / ProcessPendingAssets so the worker
	// pool isn't multiplexed across three batch workloads at once.
	bm.heavyOpMu.Lock()
	defer bm.heavyOpMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in SyncWallet: %v", r)
			slog.Error("panic in SyncWallet", "panic", r)
		}
		// Clear progress on completion
		bm.updateProgress(func(p *SyncProgress) {
			p.IsActive = false
			p.Phase = "idle"
			p.Message = "Sync complete"
		})
	}()

	// Initialize progress
	bm.updateProgress(func(p *SyncProgress) {
		p.IsActive = true
		p.Phase = "fetching"
		p.WalletAddress = address
		p.TotalNFTs = 0
		p.ProcessedNFTs = 0
		p.TotalAssets = 0
		p.PinnedAssets = 0
		p.FailedAssets = 0
		p.StartedAt = time.Now()
		p.Message = "Fetching NFTs from blockchain..."
	})

	// Reset processed URIs for this sync
	bm.processedURIs.Store(new(sync.Map))

	// Get wallet to check last synced level for incremental sync
	wallet, err := bm.db.GetWallet(address)
	if err != nil {
		return 0, fmt.Errorf("failed to get wallet: %w", err)
	}
	sinceLevel := wallet.LastSyncedLevel

	// Get current head level BEFORE fetching - this ensures we don't miss any updates
	currentHead, err := bm.indexer.GetHead(ctx)
	if err != nil {
		slog.Warn("failed to get head level, doing full sync", "error", err)
		sinceLevel = 0
		currentHead = 0
	}

	if sinceLevel > 0 {
		slog.Info("starting incremental sync", "wallet", address, "since_level", sinceLevel, "head", currentHead)
	} else {
		slog.Info("starting full sync", "wallet", address, "head", currentHead)
	}

	var ownedTokens, createdTokens []indexer.Token

	// 1. Fetch owned NFTs (if enabled for this wallet)
	if wallet.SyncOwned {
		bm.updateProgress(func(p *SyncProgress) {
			if sinceLevel > 0 {
				p.Message = "Fetching new owned NFTs..."
			} else {
				p.Message = "Fetching owned NFTs..."
			}
		})
		var err error
		ownedTokens, err = bm.indexer.SyncOwnedSince(ctx, address, sinceLevel)
		if err != nil {
			return 0, fmt.Errorf("failed to sync owned tokens: %w", err)
		}
	} else {
		slog.Info("skipping owned NFTs", "wallet", address, "reason", "disabled")
	}

	// 2. Fetch created NFTs (if enabled for this wallet)
	if wallet.SyncCreated {
		bm.updateProgress(func(p *SyncProgress) {
			if sinceLevel > 0 {
				p.Message = "Fetching new created NFTs..."
			} else {
				p.Message = "Fetching created NFTs..."
			}
		})
		var err error
		createdTokens, err = bm.indexer.SyncCreatedSince(ctx, address, sinceLevel)
		if err != nil {
			return 0, fmt.Errorf("failed to sync created tokens: %w", err)
		}
	} else {
		slog.Info("skipping created NFTs", "wallet", address, "reason", "disabled")
	}

	// Check if anything to sync
	if !wallet.SyncOwned && !wallet.SyncCreated {
		slog.Info("both sync options disabled", "wallet", address)
		return currentHead, nil
	}

	// 3. Combine and deduplicate NFTs
	allTokens := append(ownedTokens, createdTokens...)
	tokenMap := make(map[string]indexer.Token)
	for _, token := range allTokens {
		key := fmt.Sprintf("%s:%s", token.Contract.Address, token.TokenID)
		tokenMap[key] = token
	}

	// 4. Collect all unique IPFS asset URIs across all NFTs
	assetURIs := make(map[string]bool)
	for _, token := range tokenMap {
		if token.Metadata != nil {
			collectAssetURIs(token.Metadata, assetURIs)
		}
	}
	totalAssets := len(assetURIs)

	bm.updateProgress(func(p *SyncProgress) {
		p.Phase = "processing"
		p.TotalNFTs = len(tokenMap)
		p.TotalAssets = totalAssets
		p.Message = fmt.Sprintf("Processing %d NFTs with %d unique assets...", len(tokenMap), totalAssets)
	})

	slog.Info("found NFTs for sync", "wallet", address, "nfts", len(tokenMap), "assets", totalAssets)

	// 5. Process each NFT (bounded concurrency)
	var wg sync.WaitGroup
	var processed int64
	total := int64(len(tokenMap))
	sem := make(chan struct{}, cap(bm.workers)) // match configured concurrency

	for _, token := range tokenMap {
		// Check for pause before starting new work
		if bm.IsPaused() {
			slog.Info("sync paused, stopping NFT processing")
			break
		}

		select {
		case sem <- struct{}{}: // acquire slot
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(t indexer.Token) {
			defer func() { <-sem }() // release slot
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic processing NFT", "contract", t.Contract.Address, "token", t.TokenID, "panic", r)
				}
				// Update progress
				cur := atomic.AddInt64(&processed, 1)
				bm.updateProgress(func(p *SyncProgress) {
					p.ProcessedNFTs++
					if t.Metadata != nil && cur < total {
						p.CurrentItem = t.Metadata.Name
					} else {
						p.CurrentItem = "Finishing..."
					}
				})
			}()

			// Check pause inside goroutine too
			if bm.IsPaused() {
				return
			}

			if err := bm.processNFT(ctx, address, t); err != nil {
				slog.Error("error processing NFT", "contract", t.Contract.Address, "token", t.TokenID, "error", err)
			}
		}(token)
	}

	wg.Wait()

	// Update progress to show completion
	bm.updateProgress(func(p *SyncProgress) {
		p.CurrentItem = "Complete"
		if bm.IsPaused() {
			p.Message = "Paused"
		} else {
			p.Message = fmt.Sprintf("Synced %d NFTs", total)
		}
	})

	slog.Info("sync complete", "wallet", address)
	return currentHead, nil
}

// collectAssetURIs adds all unique IPFS URIs from metadata to the seen map
func collectAssetURIs(m *indexer.TokenMetadata, seen map[string]bool) {
	if m == nil {
		return
	}

	// Artifact
	if m.ArtifactURI != "" && ipfsuri.IsIPFS(m.ArtifactURI) {
		seen[m.ArtifactURI] = true
	}

	// Display if different
	if m.DisplayURI != "" && ipfsuri.IsIPFS(m.DisplayURI) {
		seen[m.DisplayURI] = true
	}

	// Thumbnail if different
	if m.ThumbnailURI != "" && ipfsuri.IsIPFS(m.ThumbnailURI) {
		seen[m.ThumbnailURI] = true
	}

	// Formats
	for _, f := range m.Formats {
		if f.URI != "" && ipfsuri.IsIPFS(f.URI) {
			seen[f.URI] = true
		}
	}

	// Non-standard fields discovered from raw JSON
	for _, uri := range indexer.ExtractExtraIPFSURIs(m.RawJSON) {
		seen[uri] = true
	}
}

// processNFT processes a single NFT (saves to DB and backs up assets)
func (bm *BackupManager) processNFT(ctx context.Context, walletAddr string, token indexer.Token) error {
	// Check for pause or storage limits before doing any work
	if bm.IsPaused() {
		return nil
	}
	if !bm.isWithinStorageLimit() {
		bm.SetPaused(true)
		return fmt.Errorf("storage limit reached")
	}

	// Acquire worker slot (semaphore pattern)
	select {
	case bm.workers <- struct{}{}:
		defer func() { <-bm.workers }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// If metadata is nil, try to fetch it from the chain
	if token.Metadata == nil {
		metadata, err := bm.fetchMetadataFromChain(ctx, token.Contract.Address, token.TokenID)
		if err != nil {
			slog.Warn("could not fetch metadata from chain", "contract", token.Contract.Address, "token", token.TokenID, "error", err)
			// Skip this token if we can't get metadata
			return nil
		}
		token.Metadata = metadata
	}

	// Skip if still no useful content after fetching
	if token.Metadata == nil || !indexer.HasIPFSContent(token.Metadata) {
		slog.Debug("skipping NFT, no IPFS content", "contract", token.Contract.Address, "token", token.TokenID)
		return nil
	}

	// 1. Save NFT to database with full metadata
	nft := &db.NFT{
		TokenID:         token.TokenID,
		ContractAddress: token.Contract.Address,
		WalletAddress:   walletAddr,
		Name:            token.Metadata.Name,
		Description:     token.Metadata.Description,
		ArtifactURI:     token.Metadata.ArtifactURI,
		DisplayURI:      token.Metadata.DisplayURI,
		ThumbnailURI:    token.Metadata.ThumbnailURI,
	}

	// Set creator from firstMinter if available
	if token.FirstMinter != nil {
		nft.CreatorAddress = token.FirstMinter.Address
	}

	// Store full metadata JSON for future recovery (e.g., VerifyAndFixPins)
	// and for discovering non-standard IPFS URIs on re-processing.
	if len(token.Metadata.RawJSON) > 0 {
		nft.RawMetadata = string(token.Metadata.RawJSON)
	} else {
		// Fallback: store the on-chain metadata URI pointer
		rawURI, err := bm.indexer.FetchRawMetadataURI(ctx, token.Contract.Address, token.TokenID)
		if err != nil {
			slog.Warn("could not fetch raw metadata URI", "contract", token.Contract.Address, "token", token.TokenID, "error", err)
		} else {
			rawMetadata := map[string]string{"uri": rawURI}
			rawJSON, _ := json.Marshal(rawMetadata)
			nft.RawMetadata = string(rawJSON)
		}
	}

	// Preserve existing RawMetadata if we couldn't obtain a new value.
	// SaveNFT writes ALL columns, so an empty string would erase a
	// previously stored value during re-sync if metadata fetch fails.
	if nft.RawMetadata == "" {
		var existing db.NFT
		if bm.db.DB.Select("raw_metadata").
			Where("token_id = ? AND contract_address = ?", nft.TokenID, nft.ContractAddress).
			First(&existing).Error == nil && existing.RawMetadata != "" {
			nft.RawMetadata = existing.RawMetadata
		}
	}

	if err := bm.db.SaveNFT(nft); err != nil {
		return fmt.Errorf("failed to save NFT: %w", err)
	}

	// 2. Queue assets for backup with proper types
	type assetEntry struct {
		uri       string
		assetType string
	}

	var assets []assetEntry

	// Add artifact (main content)
	if token.Metadata.ArtifactURI != "" && ipfsuri.IsIPFS(token.Metadata.ArtifactURI) {
		assets = append(assets, assetEntry{token.Metadata.ArtifactURI, "artifact"})
	}

	// Add display URI if different from artifact
	if token.Metadata.DisplayURI != "" && token.Metadata.DisplayURI != token.Metadata.ArtifactURI && ipfsuri.IsIPFS(token.Metadata.DisplayURI) {
		assets = append(assets, assetEntry{token.Metadata.DisplayURI, "display"})
	}

	// Add thumbnail if different from artifact
	if token.Metadata.ThumbnailURI != "" && token.Metadata.ThumbnailURI != token.Metadata.ArtifactURI && ipfsuri.IsIPFS(token.Metadata.ThumbnailURI) {
		assets = append(assets, assetEntry{token.Metadata.ThumbnailURI, "thumbnail"})
	}

	// Add additional formats
	for _, format := range token.Metadata.Formats {
		if format.URI != "" && ipfsuri.IsIPFS(format.URI) {
			assets = append(assets, assetEntry{format.URI, "format"})
		}
	}

	// Add IPFS URIs found in non-standard metadata fields (e.g. Versum pinUri)
	standardURIs := make(map[string]bool, len(assets))
	for _, a := range assets {
		standardURIs[a.uri] = true
	}
	for _, extraURI := range indexer.ExtractExtraIPFSURIs(token.Metadata.RawJSON) {
		if !standardURIs[extraURI] {
			assets = append(assets, assetEntry{extraURI, "metadata"})
		}
	}

	for _, asset := range assets {
		if err := bm.backupAsset(ctx, nft.ID, asset.uri, asset.assetType); err != nil {
			slog.Error("failed to backup asset", "uri", asset.uri, "error", err)
		}
	}

	return nil
}

// backupAsset downloads and pins an asset to IPFS
func (bm *BackupManager) backupAsset(ctx context.Context, nftID uint64, uri string, assetType string) error {
	// Lazy-init processedURIs — handles BackupManager built via struct literal (e.g. in tests)
	// rather than NewBackupManager. CompareAndSwap is safe under concurrent access.
	if bm.processedURIs.Load() == nil {
		bm.processedURIs.CompareAndSwap(nil, new(sync.Map))
	}
	// Check if we've already processed this URI in this sync (deduplication)
	if _, loaded := bm.processedURIs.Load().LoadOrStore(uri, true); loaded {
		// Already processed in this sync, skip
		return nil
	}

	// Create or update asset record regardless of pause state
	// This prevents data loss where an NFT is processed but its assets are skipped due to pause
	existingAsset, err := bm.db.GetAssetByURI(uri)

	// If it's already pinned, we can skip early
	if err == nil && existingAsset != nil && existingAsset.Status == db.StatusPinned {
		slog.Debug("asset already pinned, skipping", "uri", uri)
		bm.updateProgress(func(p *SyncProgress) {
			p.PinnedAssets++
		})
		return nil
	}

	// If it was previously classified as not pinnable (non-IPFS URI), leave it alone
	if err == nil && existingAsset != nil && existingAsset.Status == db.StatusSkipped {
		return nil
	}

	asset := &db.Asset{
		NFTID:  nftID,
		URI:    uri,
		Status: db.StatusPending,
		Type:   assetType,
	}

	if existingAsset != nil {
		asset.ID = existingAsset.ID
		asset.RetryCount = existingAsset.RetryCount
		// If it was failed, reset to pending
		if existingAsset.Status == db.StatusFailed || existingAsset.Status == db.StatusFailedUnavailable {
			asset.Status = db.StatusPending
			asset.ErrorMsg = ""
		} else {
			asset.Status = existingAsset.Status
		}
	}

	if err := bm.db.SaveAsset(asset); err != nil {
		return fmt.Errorf("failed to save asset: %w", err)
	}

	// Check for pause AFTER saving record
	if bm.IsPaused() {
		return nil
	}

	// Non-IPFS URIs (HTTP/HTTPS) cannot be pinned — mark terminal so they are never retried
	if !ipfsuri.IsIPFS(uri) {
		slog.Debug("skipping non-IPFS URI", "uri", uri)
		asset.Status = db.StatusSkipped
		asset.ErrorMsg = ""
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save skipped status", "uri", uri, "error", err)
		}
		return nil
	}

	// Update progress phase
	bm.updateProgress(func(p *SyncProgress) {
		p.Phase = "pinning"
	})

	// Already saved above, just using the result now if needed, but we re-fetch effectively or use the object.
	// Actually we already have 'asset' object updated.

	// Check storage limit first - this is the user's hard limit
	if !bm.isWithinStorageLimit() {
		slog.Warn("storage limit reached, stopping backup")
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Storage limit reached"
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		// Auto-pause to prevent further attempts
		bm.SetPaused(true)
		return fmt.Errorf("storage limit reached")
	}

	// Check disk space
	if !bm.hasSufficientDiskSpace() {
		slog.Warn("insufficient disk space, stopping backup")
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Insufficient disk space"
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		// Auto-pause to prevent further attempts
		bm.SetPaused(true)
		return fmt.Errorf("insufficient disk space")
	}

	// Try to get file info (size, mime type) via HTTP HEAD - this is optional
	// If the gateway doesn't respond, we can still pin directly via IPFS
	_, mimeType, size, err := bm.downloadMetadata(ctx, uri)
	if err != nil {
		// Gateway didn't respond - that's fine, IPFS will fetch it directly
		slog.Debug("gateway unavailable, pinning directly via IPFS", "uri", uri)
	} else {
		// Validate size only if we got it
		if size > bm.config.IPFS.MaxFileSize {
			asset.Status = db.StatusFailed
			asset.ErrorMsg = fmt.Sprintf("File too large: %d bytes (max %d)", size, bm.config.IPFS.MaxFileSize)
			if err := bm.db.SaveAsset(asset); err != nil {
				slog.Warn("failed to save asset status", "uri", uri, "error", err)
			}
			return fmt.Errorf("file too large: %d bytes", size)
		}
		asset.SizeBytes = size
		asset.MimeType = mimeType
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset metadata", "uri", uri, "error", err)
		}
	}

	// Extract CID from URI (if it's an IPFS URI)
	cid := ExtractCIDFromURI(uri)
	if cid == "" {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Invalid IPFS URI - could not extract CID"
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		bm.updateProgress(func(p *SyncProgress) {
			p.FailedAssets++
		})
		return fmt.Errorf("could not extract CID from URI: %s", uri)
	}

	// Pin to IPFS with retry logic
	err = bm.pinWithRetry(ctx, cid, asset.RetryCount)
	if err != nil {
		asset.RetryCount++
		if isTimeoutError(err) {
			asset.Status = db.StatusFailedUnavailable
			asset.ErrorMsg = "Content not available on IPFS network (timeout)"
		} else {
			asset.Status = db.StatusFailed
			asset.ErrorMsg = err.Error()
		}
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		bm.updateProgress(func(p *SyncProgress) {
			p.FailedAssets++
		})
		return err
	}

	// Get actual size from IPFS after pinning
	if size, err := bm.getIPFS().Stat(ctx, cid); err == nil && size > 0 {
		asset.SizeBytes = size
	} else if err != nil {
		slog.Warn("could not get size after pin", "cid", cid, "error", err)
	}

	// Success
	asset.Status = db.StatusPinned
	now := time.Now()
	asset.PinnedAt = &now
	if err := bm.db.SaveAsset(asset); err != nil {
		slog.Warn("failed to save pinned status", "uri", uri, "error", err)
	}

	// Mark disk usage for update
	bm.MarkDiskUsageDirty()

	bm.updateProgress(func(p *SyncProgress) {
		p.PinnedAssets++
	})

	slog.Info("successfully pinned asset", "uri", uri, "cid", cid, "size", asset.SizeBytes)
	return nil
}

// downloadMetadata fetches metadata about an asset without downloading the full file
func (bm *BackupManager) downloadMetadata(ctx context.Context, uri string) ([]byte, string, int64, error) {
	httpURI := resolveURI(uri) // Convert ipfs:// to gateway URL

	// Create a new context with timeout to prevent infinite hangs on bad gateways
	// This is CRITICAL: the parent context might be the service context which lives forever.
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "HEAD", httpURI, nil)
	if err != nil {
		return nil, "", 0, err
	}

	resp, err := httpx.Client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", 0, fmt.Errorf("bad status: %s", resp.Status)
	}

	// Just return the info - actual size validation happens in backupAsset
	return nil, resp.Header.Get("Content-Type"), resp.ContentLength, nil
}

// VerifyAndFixPins iterates through all NFTs and ensures their assets are properly tracked and pinned
// This fixes data loss from the previous "pause bug" where assets weren't saved to DB
func (bm *BackupManager) VerifyAndFixPins(ctx context.Context) (map[string]int, error) {
	bm.heavyOpMu.Lock()
	defer bm.heavyOpMu.Unlock()

	slog.Info("starting VerifyAndFixPins")

	stats := map[string]int{
		"checked":   0,
		"processed": 0,
		"errors":    0,
	}

	// 1. Get all NFTs from DB
	// Process in batches to avoid memory issues
	limit := 100
	offset := 0

	for {
		// Check for shutdown
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}

		var nfts []db.NFT
		if err := bm.db.DB.Order("id asc").Offset(offset).Limit(limit).Find(&nfts).Error; err != nil {
			return stats, fmt.Errorf("failed to fetch NFTs: %w", err)
		}

		if len(nfts) == 0 {
			break
		}

		for _, nft := range nfts {
			select {
			case <-ctx.Done():
				return stats, ctx.Err()
			default:
			}
			stats["checked"]++

			// Reconstruct Token/Metadata from DB record.
			// If RawMetadata contains full JSON (new format), parse it to
			// recover Formats and non-standard IPFS URI fields.
			metadata := reconstructMetadata(nft)

			token := indexer.Token{
				TokenID: nft.TokenID,
				Contract: indexer.ContractInfo{
					Address: nft.ContractAddress,
				},
				Metadata: &metadata,
			}

			// Call processNFT to ensure assets are tracked
			// We use a background context or the passed context
			// We suppress errors to keep going
			if err := bm.processNFT(ctx, nft.WalletAddress, token); err != nil {
				slog.Error("VerifyAndFix: error processing NFT", "nft_id", nft.ID, "name", nft.Name, "error", err)
				stats["errors"]++
			} else {
				stats["processed"]++
			}
		}

		offset += limit
	}

	slog.Info("VerifyAndFixPins complete", "checked", stats["checked"], "processed", stats["processed"], "errors", stats["errors"])
	return stats, nil
}

// pinWithRetry pins content with exponential backoff
func (bm *BackupManager) pinWithRetry(ctx context.Context, cid string, retryCount int) error {
	maxRetries := 2 // Reduced from 3 to avoid long waits
	backoff := time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := backoff * time.Duration(1<<uint(attempt-1))
			slog.Debug("retrying pin", "attempt", attempt, "max", maxRetries, "cid", cid, "delay", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Use a shorter timeout per attempt to avoid blocking too long
		timeout := bm.config.IPFS.PinTimeout
		if timeout > 60*time.Second {
			timeout = 60 * time.Second // Cap at 60s per attempt
		}

		err := bm.getIPFS().Pin(ctx, cid, timeout)
		if err == nil {
			return nil
		}
		lastErr = err

		slog.Warn("pin attempt failed", "attempt", attempt+1, "cid", cid, "error", err)
	}

	// Preserve the underlying cause (e.g. a timeout when the content is not
	// retrievable on the IPFS network) so callers can classify the failure
	// correctly instead of treating every exhausted retry as a generic error.
	return fmt.Errorf("max retries exceeded for CID %s: %w", cid, lastErr)
}

// isWithinStorageLimit checks if we're within the user's configured storage limit
func (bm *BackupManager) isWithinStorageLimit() bool {
	maxGB := bm.config.Backup.MaxStorageGB
	if maxGB <= 0 {
		return true // No limit set
	}

	// Get current storage usage from database
	stats, err := bm.db.GetAssetStats()
	if err != nil {
		slog.Error("failed to get storage stats", "error", err)
		return true // Fail open
	}

	usedBytes := stats["total_size_bytes"]
	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)

	if usedGB >= float64(maxGB) {
		slog.Warn("storage limit reached", "used_gb", usedGB, "limit_gb", maxGB)
		return false
	}

	return true
}

// ExtractCIDFromURI extracts a CID from an IPFS URI
// Handles: ipfs://CID, ipfs://CID/path, ipfs://CID?query, /ipfs/CID, etc.
func ExtractCIDFromURI(uri string) string {
	var cid string

	// Handle ipfs:// scheme
	if len(uri) > 7 && uri[:7] == "ipfs://" {
		cid = uri[7:]
	} else {
		// Find /ipfs/ in the URI (for gateway URLs)
		const ipfsPrefix = "/ipfs/"
		idx := strings.Index(uri, ipfsPrefix)
		if idx != -1 {
			cid = uri[idx+len(ipfsPrefix):]
		}
	}

	if cid == "" {
		return ""
	}

	// Strip query parameters (e.g., ?fxhash=...)
	if qIdx := strings.Index(cid, "?"); qIdx != -1 {
		cid = cid[:qIdx]
	}

	// Strip trailing path (e.g., /index.html) - keep only the CID
	if slashIdx := strings.Index(cid, "/"); slashIdx != -1 {
		cid = cid[:slashIdx]
	}

	return cid
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Matches both a bare deadline error and one wrapped by pinWithRetry
	// (e.g. "max retries exceeded for CID ...: ...: context deadline exceeded").
	return errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "context deadline exceeded")
}

// Shutdown gracefully shuts down the backup manager
func (bm *BackupManager) Shutdown() {
	// Update disk usage one final time before shutdown
	bm.UpdateDiskUsage()
}

// MarkDiskUsageDirty marks that disk usage needs recalculation
func (bm *BackupManager) MarkDiskUsageDirty() {
	atomic.StoreInt32(&bm.diskUsageDirty, 1)
}

// UpdateDiskUsage recalculates disk usage if dirty and saves to DB
func (bm *BackupManager) UpdateDiskUsage() {
	if atomic.CompareAndSwapInt32(&bm.diskUsageDirty, 1, 0) {
		repoPath := bm.getIPFS().GetRepoPath()
		sizeBytes, err := GetDiskUsageBytes(repoPath)
		if err != nil {
			slog.Error("failed to get disk usage", "error", err)
			return
		}

		bm.db.SetSetting("disk_usage_bytes", fmt.Sprintf("%d", sizeBytes))
		slog.Info("updated disk usage", "gb", float64(sizeBytes)/1024/1024/1024)
	}
}

// resolveURI converts IPFS URIs to HTTP gateway URIs for metadata checking
func resolveURI(uri string) string {
	if strings.HasPrefix(uri, "ipfs://") {
		return "https://ipfs.io/ipfs/" + uri[7:]
	}
	return uri
}

// reconstructMetadata rebuilds a TokenMetadata from a DB record.
// If RawMetadata contains full metadata JSON (new format), it is parsed to
// recover Formats and non-standard fields. Old records storing only
// {"uri": "ipfs://..."} fall back to column-based reconstruction.
func reconstructMetadata(nft db.NFT) indexer.TokenMetadata {
	if nft.RawMetadata != "" {
		// Detect old format: a single-key object like {"uri": "ipfs://..."}
		var probe map[string]json.RawMessage
		if json.Unmarshal([]byte(nft.RawMetadata), &probe) == nil {
			if _, hasURI := probe["uri"]; !(hasURI && len(probe) == 1) {
				// Looks like full metadata JSON — parse it
				var metadata indexer.TokenMetadata
				if json.Unmarshal([]byte(nft.RawMetadata), &metadata) == nil {
					return metadata
				}
			}
		}
	}

	// Fallback: reconstruct from DB columns (no Formats or non-standard fields)
	return indexer.TokenMetadata{
		Name:         nft.Name,
		Description:  nft.Description,
		ArtifactURI:  nft.ArtifactURI,
		DisplayURI:   nft.DisplayURI,
		ThumbnailURI: nft.ThumbnailURI,
	}
}

// fetchMetadataFromChain fetches token metadata from the blockchain when TZKT doesn't have it
func (bm *BackupManager) fetchMetadataFromChain(ctx context.Context, contractAddr, tokenID string) (*indexer.TokenMetadata, error) {
	// First try to get the raw metadata URI from the contract's bigmap
	rawURI, err := bm.indexer.FetchRawMetadataURI(ctx, contractAddr, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch raw URI: %w", err)
	}

	// The raw URI should be an IPFS link to the metadata JSON
	if !strings.HasPrefix(rawURI, "ipfs://") && !strings.Contains(rawURI, "/ipfs/") {
		return nil, fmt.Errorf("raw URI is not IPFS: %s", rawURI)
	}

	// Resolve to HTTP gateway and fetch the JSON
	httpURL := resolveURI(rawURI)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", httpURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpx.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching metadata", resp.StatusCode)
	}

	var metadata indexer.TokenMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode metadata JSON: %w", err)
	}

	return &metadata, nil
}

// PinAssetByID pins a specific asset by its database ID
// This is used for immediate retry/repin operations
func (bm *BackupManager) PinAssetByID(ctx context.Context, assetID uint64) error {
	// Get the asset from database
	var asset db.Asset
	if err := bm.db.DB.Preload("NFT").First(&asset, assetID).Error; err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	// Reset status to pending
	asset.Status = db.StatusPending
	asset.RetryCount = 0
	asset.ErrorMsg = ""
	if err := bm.db.SaveAsset(&asset); err != nil {
		return fmt.Errorf("failed to reset asset status: %w", err)
	}

	// Pin it directly using backupAsset logic
	return bm.pinAssetDirect(ctx, &asset)
}

// pinAssetDirect pins a specific asset directly (for retry operations)
func (bm *BackupManager) pinAssetDirect(ctx context.Context, asset *db.Asset) error {
	uri := asset.URI

	// Non-IPFS URIs cannot be pinned — mark terminal so they are never retried
	if !strings.HasPrefix(uri, "ipfs://") && !strings.Contains(uri, "/ipfs/") {
		asset.Status = db.StatusSkipped
		asset.ErrorMsg = ""
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save skipped status", "uri", uri, "error", err)
		}
		return errAssetSkipped
	}

	// Check storage limit
	if !bm.isWithinStorageLimit() {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Storage limit reached"
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		return fmt.Errorf("storage limit reached")
	}

	// Check disk space
	if !bm.hasSufficientDiskSpace() {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Insufficient disk space"
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		return fmt.Errorf("insufficient disk space")
	}

	// Try to get file info via HTTP HEAD
	_, mimeType, size, err := bm.downloadMetadata(ctx, uri)
	if err == nil {
		if size > bm.config.IPFS.MaxFileSize {
			asset.Status = db.StatusFailed
			asset.ErrorMsg = fmt.Sprintf("File too large: %d bytes", size)
			if err := bm.db.SaveAsset(asset); err != nil {
				slog.Warn("failed to save asset status", "uri", uri, "error", err)
			}
			return fmt.Errorf("file too large")
		}
		asset.SizeBytes = size
		asset.MimeType = mimeType
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset metadata", "uri", uri, "error", err)
		}
	}

	// Extract CID from URI
	cid := ExtractCIDFromURI(uri)
	if cid == "" {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Invalid IPFS URI - could not extract CID"
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		return fmt.Errorf("could not extract CID from URI: %s", uri)
	}

	// Pin to IPFS
	err = bm.pinWithRetry(ctx, cid, 0)
	if err != nil {
		asset.RetryCount++
		if isTimeoutError(err) {
			asset.Status = db.StatusFailedUnavailable
			asset.ErrorMsg = "Content not available on IPFS network (timeout)"
		} else {
			asset.Status = db.StatusFailed
			asset.ErrorMsg = err.Error()
		}
		if err := bm.db.SaveAsset(asset); err != nil {
			slog.Warn("failed to save asset status", "uri", uri, "error", err)
		}
		return err
	}

	// Get actual size from IPFS after pinning
	if size, err := bm.getIPFS().Stat(ctx, cid); err == nil && size > 0 {
		asset.SizeBytes = size
	}

	// Success
	asset.Status = db.StatusPinned
	now := time.Now()
	asset.PinnedAt = &now
	if err := bm.db.SaveAsset(asset); err != nil {
		slog.Warn("failed to save pinned status", "uri", uri, "error", err)
	}

	// Mark disk usage for update
	bm.MarkDiskUsageDirty()

	slog.Info("successfully pinned asset", "uri", uri, "cid", cid)
	return nil
}

// ProcessPendingAssets processes all assets stuck in pending status.
// This is used to resume interrupted syncs or manually retry pending work.
func (bm *BackupManager) ProcessPendingAssets(ctx context.Context, limit int) (processed int, pinned int, failed int) {
	bm.heavyOpMu.Lock()
	defer bm.heavyOpMu.Unlock()

	assets, err := bm.db.GetPendingAssets(limit)
	if err != nil {
		slog.Error("failed to get pending assets", "error", err)
		return 0, 0, 0
	}

	if len(assets) == 0 {
		return 0, 0, 0
	}

	slog.Info("processing pending assets", "count", len(assets))

	var wg sync.WaitGroup
	var pCount, pinCount, fCount int64

	// Bound goroutine creation to the configured worker count. Spawning one
	// goroutine per asset (up to `limit`) and letting them all block on
	// bm.workers piled goroutines + their captured asset state in memory on
	// small machines, contributing to OOM kills.
	sem := make(chan struct{}, cap(bm.workers))

dispatch:
	for _, asset := range assets {
		// Early exit checks
		if ctx.Err() != nil {
			break
		}

		if bm.IsPaused() {
			slog.Info("paused, stopping pending asset processing")
			break
		}

		// Block here until a slot is free — caps in-flight goroutines.
		// A labelled break is required because a plain `break` inside a
		// select would only exit the select, not the dispatch loop.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break dispatch
		}

		wg.Add(1)
		// Capture loop variable
		go func(a db.Asset) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic processing pending asset", "uri", a.URI, "panic", r)
					atomic.AddInt64(&fCount, 1)
				}
			}()

			// Re-check pause now that we hold a slot
			if bm.IsPaused() {
				return
			}

			atomic.AddInt64(&pCount, 1)
			err := bm.pinAssetDirect(ctx, &a)
			if err != nil {
				if !errors.Is(err, errAssetSkipped) {
					atomic.AddInt64(&fCount, 1)
					slog.Error("failed to pin pending asset", "uri", a.URI, "error", err)
				}
				// errAssetSkipped: asset is now marked terminal in DB; not a failure
			} else {
				atomic.AddInt64(&pinCount, 1)
			}
		}(asset)
	}

	wg.Wait()

	// Update disk usage after processing
	bm.UpdateDiskUsage()

	return int(atomic.LoadInt64(&pCount)), int(atomic.LoadInt64(&pinCount)), int(atomic.LoadInt64(&fCount))
}
