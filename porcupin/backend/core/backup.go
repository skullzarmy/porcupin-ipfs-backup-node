package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"porcupin/backend/config"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
)

// SyncProgress represents the current sync operation progress
type SyncProgress struct {
	IsActive      bool      `json:"is_active"`
	Phase         string    `json:"phase"`           // "idle", "fetching", "processing", "pinning"
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

// BackupManager orchestrates the backup process
type BackupManager struct {
	ipfs     *ipfs.Node
	indexer  *indexer.Indexer
	db       *db.Database
	config   *config.Config
	mu       sync.RWMutex
	workers  chan struct{}
	shutdown chan struct{}
	
	// Pause control
	pauseMu  sync.RWMutex
	isPaused bool
	
	// Sync progress tracking
	progressMu    sync.RWMutex
	progress      SyncProgress
	processedURIs sync.Map // tracks URIs processed in current sync to avoid double-counting
	
	// Disk usage tracking - update after pins, not on every pin
	diskUsageDirty int32 // atomic flag: 1 if pins happened since last du
}

// NewBackupManager creates a new backup manager
func NewBackupManager(ipfsNode *ipfs.Node, idx *indexer.Indexer, database *db.Database, cfg *config.Config) *BackupManager {
	return &BackupManager{
		ipfs:     ipfsNode,
		indexer:  idx,
		db:       database,
		config:   cfg,
		workers:  make(chan struct{}, cfg.Backup.MaxConcurrency),
		shutdown: make(chan struct{}),
		progress: SyncProgress{Phase: "idle"},
	}
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
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in SyncWallet: %v", r)
			log.Printf("Panic in SyncWallet: %v", r)
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
	bm.processedURIs = sync.Map{}

	// Get wallet to check last synced level for incremental sync
	wallet, err := bm.db.GetWallet(address)
	if err != nil {
		return 0, fmt.Errorf("failed to get wallet: %w", err)
	}
	sinceLevel := wallet.LastSyncedLevel

	// Get current head level BEFORE fetching - this ensures we don't miss any updates
	currentHead, err := bm.indexer.GetHead(ctx)
	if err != nil {
		log.Printf("Warning: failed to get head level, doing full sync: %v", err)
		sinceLevel = 0
		currentHead = 0
	}

	if sinceLevel > 0 {
		log.Printf("Starting incremental sync for wallet: %s (since level %d, current head %d)", address, sinceLevel, currentHead)
	} else {
		log.Printf("Starting full sync for wallet: %s (current head %d)", address, currentHead)
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
		log.Printf("Skipping owned NFTs for wallet %s (disabled)", address)
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
		log.Printf("Skipping created NFTs for wallet %s (disabled)", address)
	}

	// Check if anything to sync
	if !wallet.SyncOwned && !wallet.SyncCreated {
		log.Printf("Both sync options disabled for wallet %s, nothing to do", address)
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

	log.Printf("Found %d unique NFTs with %d unique assets for %s", len(tokenMap), totalAssets, address)

	// 5. Process each NFT
	var wg sync.WaitGroup
	processed := 0
	total := len(tokenMap)
	
	for _, token := range tokenMap {
		// Check for pause before starting new work
		if bm.IsPaused() {
			log.Printf("Sync paused, stopping NFT processing")
			break
		}
		
		wg.Add(1)
		go func(t indexer.Token) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Panic processing NFT %s:%s - %v", t.Contract.Address, t.TokenID, r)
				}
				// Update progress
				bm.updateProgress(func(p *SyncProgress) {
					p.ProcessedNFTs++
					processed++
					if t.Metadata != nil && processed < total {
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
				log.Printf("Error processing NFT %s:%s - %v", t.Contract.Address, t.TokenID, err)
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
	
	log.Printf("Sync complete for wallet: %s", address)
	return currentHead, nil
}

// countAssets counts how many IPFS assets are in token metadata
func countAssets(m *indexer.TokenMetadata) int {
	if m == nil {
		return 0
	}
	
	seen := make(map[string]bool)
	collectAssetURIs(m, seen)
	return len(seen)
}

// collectAssetURIs adds all unique IPFS URIs from metadata to the seen map
func collectAssetURIs(m *indexer.TokenMetadata, seen map[string]bool) {
	if m == nil {
		return
	}
	
	// Artifact
	if m.ArtifactURI != "" && isIPFSURI(m.ArtifactURI) {
		seen[m.ArtifactURI] = true
	}
	
	// Display if different
	if m.DisplayURI != "" && isIPFSURI(m.DisplayURI) {
		seen[m.DisplayURI] = true
	}
	
	// Thumbnail if different
	if m.ThumbnailURI != "" && isIPFSURI(m.ThumbnailURI) {
		seen[m.ThumbnailURI] = true
	}
	
	// Formats
	for _, f := range m.Formats {
		if f.URI != "" && isIPFSURI(f.URI) {
			seen[f.URI] = true
		}
	}
}

// isIPFSURI checks if a URI is an IPFS URI
func isIPFSURI(uri string) bool {
	return strings.HasPrefix(uri, "ipfs://") || strings.Contains(uri, "/ipfs/")
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
	case <-bm.shutdown:
		return fmt.Errorf("backup manager shutdown")
	}

	// If metadata is nil, try to fetch it from the chain
	if token.Metadata == nil {
		metadata, err := bm.fetchMetadataFromChain(ctx, token.Contract.Address, token.TokenID)
		if err != nil {
			log.Printf("Could not fetch metadata from chain for %s:%s - %v", token.Contract.Address, token.TokenID, err)
			// Skip this token if we can't get metadata
			return nil
		}
		token.Metadata = metadata
	}

	// Skip if still no useful content after fetching
	if token.Metadata == nil || !hasIPFSContent(token.Metadata) {
		log.Printf("Skipping %s:%s - no IPFS content found", token.Contract.Address, token.TokenID)
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

	// Try to fetch raw metadata URI
	rawURI, err := bm.indexer.FetchRawMetadataURI(ctx, token.Contract.Address, token.TokenID)
	if err != nil {
		log.Printf("Could not fetch raw metadata URI for %s:%s - %v", token.Contract.Address, token.TokenID, err)
	} else {
		// Save raw metadata as JSON
		rawMetadata := map[string]string{"uri": rawURI}
		rawJSON, _ := json.Marshal(rawMetadata)
		nft.RawMetadata = string(rawJSON)
	}

	if err := bm.db.SaveNFT(nft); err != nil {
		return fmt.Errorf("failed to save NFT: %w", err)
	}

	// 2. Queue assets for backup with proper types
	type assetEntry struct {
		uri      string
		assetType string
	}
	
	var assets []assetEntry
	
	// Add artifact (main content)
	if token.Metadata.ArtifactURI != "" {
		assets = append(assets, assetEntry{token.Metadata.ArtifactURI, "artifact"})
	}
	
	// Add display URI if different from artifact
	if token.Metadata.DisplayURI != "" && token.Metadata.DisplayURI != token.Metadata.ArtifactURI {
		assets = append(assets, assetEntry{token.Metadata.DisplayURI, "display"})
	}
	
	// Add thumbnail if different from artifact
	if token.Metadata.ThumbnailURI != "" && token.Metadata.ThumbnailURI != token.Metadata.ArtifactURI {
		assets = append(assets, assetEntry{token.Metadata.ThumbnailURI, "thumbnail"})
	}
	
	// Add additional formats
	for _, format := range token.Metadata.Formats {
		if format.URI != "" {
			assets = append(assets, assetEntry{format.URI, "format"})
		}
	}

	for _, asset := range assets {
		if err := bm.backupAsset(ctx, nft.ID, asset.uri, asset.assetType); err != nil {
			log.Printf("Failed to backup asset %s - %v", asset.uri, err)
		}
	}

	return nil
}

// backupAsset downloads and pins an asset to IPFS
func (bm *BackupManager) backupAsset(ctx context.Context, nftID uint64, uri string, assetType string) error {
	// Check for pause first
	if bm.IsPaused() {
		return nil
	}

	// Skip non-IPFS URIs - we can only pin IPFS content
	if !strings.HasPrefix(uri, "ipfs://") && !strings.Contains(uri, "/ipfs/") {
		log.Printf("Skipping non-IPFS URI: %s", uri)
		return nil
	}
	
	// Check if we've already processed this URI in this sync (deduplication)
	if _, loaded := bm.processedURIs.LoadOrStore(uri, true); loaded {
		// Already processed in this sync, skip
		return nil
	}

	// Update progress phase
	bm.updateProgress(func(p *SyncProgress) {
		p.Phase = "pinning"
	})

	// Check if asset already exists and is pinned
	existingAsset, err := bm.db.GetAssetByURI(uri)
	if err == nil && existingAsset != nil && existingAsset.Status == db.StatusPinned {
		log.Printf("Asset %s already pinned, skipping", uri)
		bm.updateProgress(func(p *SyncProgress) {
			p.PinnedAssets++
		})
		return nil
	}

	// Create or update asset record
	asset := &db.Asset{
		NFTID:  nftID,
		URI:    uri,
		Status: db.StatusPending,
		Type:   assetType,
	}

	if existingAsset != nil {
		asset.ID = existingAsset.ID
		asset.RetryCount = existingAsset.RetryCount
	}

	if err := bm.db.SaveAsset(asset); err != nil {
		return fmt.Errorf("failed to save asset: %w", err)
	}

	// Check storage limit first - this is the user's hard limit
	if !bm.isWithinStorageLimit() {
		log.Printf("Storage limit reached, stopping backup")
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Storage limit reached"
		bm.db.SaveAsset(asset)
		// Auto-pause to prevent further attempts
		bm.SetPaused(true)
		return fmt.Errorf("storage limit reached")
	}

	// Check disk space
	if !bm.hasSufficientDiskSpace() {
		log.Printf("Insufficient disk space, stopping backup")
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Insufficient disk space"
		bm.db.SaveAsset(asset)
		// Auto-pause to prevent further attempts
		bm.SetPaused(true)
		return fmt.Errorf("insufficient disk space")
	}

	// Try to get file info (size, mime type) via HTTP HEAD - this is optional
	// If the gateway doesn't respond, we can still pin directly via IPFS
	_, mimeType, size, err := bm.downloadMetadata(ctx, uri)
	if err != nil {
		// Gateway didn't respond - that's fine, IPFS will fetch it directly
		log.Printf("Gateway unavailable for %s, pinning directly via IPFS", uri)
	} else {
		// Validate size only if we got it
		if size > bm.config.IPFS.MaxFileSize {
			asset.Status = db.StatusFailed
			asset.ErrorMsg = fmt.Sprintf("File too large: %d bytes (max %d)", size, bm.config.IPFS.MaxFileSize)
			bm.db.SaveAsset(asset)
			return fmt.Errorf("file too large: %d bytes", size)
		}
		asset.SizeBytes = size
		asset.MimeType = mimeType
		bm.db.SaveAsset(asset)
	}

	// Extract CID from URI (if it's an IPFS URI)
	cid := extractCIDFromURI(uri)
	if cid == "" {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Invalid IPFS URI - could not extract CID"
		bm.db.SaveAsset(asset)
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
		bm.db.SaveAsset(asset)
		bm.updateProgress(func(p *SyncProgress) {
			p.FailedAssets++
		})
		return err
	}

	// Get actual size from IPFS after pinning
	if size, err := bm.ipfs.Stat(ctx, cid); err == nil && size > 0 {
		asset.SizeBytes = size
	} else if err != nil {
		log.Printf("Could not get size for %s: %v", cid, err)
	}

	// Success
	asset.Status = db.StatusPinned
	now := time.Now()
	asset.PinnedAt = &now
	bm.db.SaveAsset(asset)
	
	// Mark disk usage for update
	bm.MarkDiskUsageDirty()

	bm.updateProgress(func(p *SyncProgress) {
		p.PinnedAssets++
	})

	log.Printf("Successfully pinned asset: %s (CID: %s, size: %d bytes)", uri, cid, asset.SizeBytes)
	return nil
}

// downloadMetadata fetches metadata about an asset without downloading the full file
func (bm *BackupManager) downloadMetadata(ctx context.Context, uri string) ([]byte, string, int64, error) {
	resolvedURI := resolveURI(uri)
	req, err := http.NewRequestWithContext(ctx, "HEAD", resolvedURI, nil)
	if err != nil {
		return nil, "", 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	mimeType := resp.Header.Get("Content-Type")
	size := resp.ContentLength

	// Just return the info - actual size validation happens in backupAsset
	return []byte{}, mimeType, size, nil
}

// pinWithRetry pins content with exponential backoff
func (bm *BackupManager) pinWithRetry(ctx context.Context, cid string, retryCount int) error {
	maxRetries := 2  // Reduced from 3 to avoid long waits
	backoff := time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := backoff * time.Duration(1<<uint(attempt-1))
			log.Printf("Retry %d/%d for CID %s after %v", attempt, maxRetries, cid, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Use a shorter timeout per attempt to avoid blocking too long
		timeout := bm.config.IPFS.PinTimeout
		if timeout > 60*time.Second {
			timeout = 60 * time.Second  // Cap at 60s per attempt
		}
		
		err := bm.ipfs.Pin(ctx, cid, timeout)
		if err == nil {
			return nil
		}

		log.Printf("Pin attempt %d failed for %s: %v", attempt+1, cid, err)
	}

	return fmt.Errorf("max retries exceeded for CID %s", cid)
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
		log.Printf("Failed to get storage stats: %v", err)
		return true // Fail open
	}

	usedBytes := stats["total_size_bytes"]
	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)

	if usedGB >= float64(maxGB) {
		log.Printf("Storage limit reached: %.2f GB used (limit: %d GB)", usedGB, maxGB)
		return false
	}

	return true
}

// extractCIDFromURI extracts a CID from an IPFS URI
// Handles: ipfs://CID, ipfs://CID/path, ipfs://CID?query, /ipfs/CID, etc.
func extractCIDFromURI(uri string) string {
	var cid string

	// Handle ipfs:// scheme
	if len(uri) > 7 && uri[:7] == "ipfs://" {
		cid = uri[7:]
	} else {
		// Find /ipfs/ in the URI (for gateway URLs)
		const ipfsPrefix = "/ipfs/"
		idx := indexOf(uri, ipfsPrefix)
		if idx != -1 {
			cid = uri[idx+len(ipfsPrefix):]
		}
	}

	if cid == "" {
		return ""
	}

	// Strip query parameters (e.g., ?fxhash=...)
	if qIdx := indexOf(cid, "?"); qIdx != -1 {
		cid = cid[:qIdx]
	}

	// Strip trailing path (e.g., /index.html) - keep only the CID
	if slashIdx := indexOf(cid, "/"); slashIdx != -1 {
		cid = cid[:slashIdx]
	}

	return cid
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return err == context.DeadlineExceeded || err.Error() == "context deadline exceeded"
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Shutdown gracefully shuts down the backup manager
func (bm *BackupManager) Shutdown() {
	// Update disk usage one final time before shutdown
	bm.UpdateDiskUsage()
	close(bm.shutdown)
}

// MarkDiskUsageDirty marks that disk usage needs recalculation
func (bm *BackupManager) MarkDiskUsageDirty() {
	atomic.StoreInt32(&bm.diskUsageDirty, 1)
}

// UpdateDiskUsage recalculates disk usage if dirty and saves to DB
func (bm *BackupManager) UpdateDiskUsage() {
	if atomic.CompareAndSwapInt32(&bm.diskUsageDirty, 1, 0) {
		repoPath := bm.ipfs.GetRepoPath()
		cmd := exec.Command("du", "-sk", repoPath)
		output, err := cmd.Output()
		if err != nil {
			log.Printf("Failed to get disk usage: %v", err)
			return
		}
		
		var sizeKB int64
		if _, err := fmt.Sscanf(string(output), "%d", &sizeKB); err != nil {
			log.Printf("Failed to parse disk usage output: %v", err)
			return
		}
		sizeBytes := sizeKB * 1024
		
		bm.db.SetSetting("disk_usage_bytes", fmt.Sprintf("%d", sizeBytes))
		log.Printf("Updated disk usage: %.2f GB", float64(sizeBytes)/1024/1024/1024)
	}
}

// resolveURI converts IPFS URIs to HTTP gateway URIs for metadata checking
func resolveURI(uri string) string {
	if strings.HasPrefix(uri, "ipfs://") {
		return "https://ipfs.io/ipfs/" + uri[7:]
	}
	return uri
}

// hasIPFSContent checks if metadata contains any IPFS URIs to backup
func hasIPFSContent(m *indexer.TokenMetadata) bool {
	if m == nil {
		return false
	}
	// Check main URIs
	if m.ArtifactURI != "" || m.DisplayURI != "" || m.ThumbnailURI != "" {
		return true
	}
	// Check formats array
	for _, f := range m.Formats {
		if f.URI != "" {
			return true
		}
	}
	return false
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
	
	resp, err := http.DefaultClient.Do(req)
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

	// Skip non-IPFS URIs
	if !strings.HasPrefix(uri, "ipfs://") && !strings.Contains(uri, "/ipfs/") {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Not an IPFS URI"
		bm.db.SaveAsset(asset)
		return fmt.Errorf("not an IPFS URI: %s", uri)
	}

	// Check storage limit
	if !bm.isWithinStorageLimit() {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Storage limit reached"
		bm.db.SaveAsset(asset)
		return fmt.Errorf("storage limit reached")
	}

	// Check disk space
	if !bm.hasSufficientDiskSpace() {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Insufficient disk space"
		bm.db.SaveAsset(asset)
		return fmt.Errorf("insufficient disk space")
	}

	// Try to get file info via HTTP HEAD
	_, mimeType, size, err := bm.downloadMetadata(ctx, uri)
	if err == nil {
		if size > bm.config.IPFS.MaxFileSize {
			asset.Status = db.StatusFailed
			asset.ErrorMsg = fmt.Sprintf("File too large: %d bytes", size)
			bm.db.SaveAsset(asset)
			return fmt.Errorf("file too large")
		}
		asset.SizeBytes = size
		asset.MimeType = mimeType
		bm.db.SaveAsset(asset)
	}

	// Extract CID from URI
	cid := extractCIDFromURI(uri)
	if cid == "" {
		asset.Status = db.StatusFailed
		asset.ErrorMsg = "Invalid IPFS URI - could not extract CID"
		bm.db.SaveAsset(asset)
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
		bm.db.SaveAsset(asset)
		return err
	}

	// Get actual size from IPFS after pinning
	if size, err := bm.ipfs.Stat(ctx, cid); err == nil && size > 0 {
		asset.SizeBytes = size
	}

	// Success
	asset.Status = db.StatusPinned
	now := time.Now()
	asset.PinnedAt = &now
	bm.db.SaveAsset(asset)

	log.Printf("Successfully pinned asset: %s (CID: %s)", uri, cid)
	return nil
}
