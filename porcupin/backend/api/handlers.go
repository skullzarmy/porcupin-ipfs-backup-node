package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/ipfs"
)

// Handlers holds the API handlers and their dependencies
type Handlers struct {
	db       *db.Database
	service  *core.BackupService
	ipfs     *ipfs.Node
	dataDir  string
	version  string
}

// NewHandlers creates a new Handlers instance
func NewHandlers(database *db.Database, service *core.BackupService, dataDir, version string) *Handlers {
	return &Handlers{
		db:      database,
		service: service,
		dataDir: dataDir,
		version: version,
	}
}

// SetIPFS sets the IPFS node for handlers that need direct access
func (h *Handlers) SetIPFS(node *ipfs.Node) {
	h.ipfs = node
}

// =============================================================================
// System Endpoints
// =============================================================================

// GetHealth returns the server health status
// GET /api/v1/health
func (h *Handlers) GetHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{
		"status":    "ok",
		"version":   h.version,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	WriteJSONRaw(w, http.StatusOK, resp)
}

// GetVersion returns the server version info
// GET /api/v1/version
func (h *Handlers) GetVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{
		"version": h.version,
	}
	WriteJSON(w, http.StatusOK, resp)
}

// GetStatus returns the sync status and service state
// GET /api/v1/status
func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		WriteServiceUnavailable(w, "backup service not available")
		return
	}

	status := h.service.GetStatus()
	WriteJSON(w, http.StatusOK, status)
}

// =============================================================================
// Statistics Endpoints
// =============================================================================

// StatsResponse is the response for the stats endpoint
type StatsResponse struct {
	TotalNFTs     int64   `json:"total_nfts"`
	TotalAssets   int64   `json:"total_assets"`
	PinnedAssets  int64   `json:"pinned_assets"`
	PendingAssets int64   `json:"pending_assets"`
	FailedAssets  int64   `json:"failed_assets"`
	StorageUsedGB float64 `json:"storage_used_gb"`
	WalletsCount  int     `json:"wallets_count"`
	ServiceState  string  `json:"service_state"`
	LastSyncAt    *string `json:"last_sync_at,omitempty"`
}

// GetStats returns current statistics
// GET /api/v1/stats
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetAssetStats()
	if err != nil {
		WriteInternalError(w, "failed to get stats: "+err.Error())
		return
	}

	wallets, err := h.db.GetAllWallets()
	if err != nil {
		WriteInternalError(w, "failed to get wallets: "+err.Error())
		return
	}

	// Get disk usage
	storageBytes, err := core.GetDiskUsageBytes(h.dataDir + "/ipfs")
	storageGB := 0.0
	if err == nil {
		storageGB = float64(storageBytes) / (1024 * 1024 * 1024)
	}

	// Get service status
	var serviceState string
	var lastSync *string
	if h.service != nil {
		status := h.service.GetStatus()
		serviceState = string(status.State)
		if status.LastSyncAt != nil {
			t := status.LastSyncAt.UTC().Format(time.RFC3339)
			lastSync = &t
		}
	}

	resp := StatsResponse{
		TotalNFTs:     stats["nft_count"],
		TotalAssets:   stats["pending"] + stats["pinned"] + stats["failed"] + stats["failed_unavailable"],
		PinnedAssets:  stats["pinned"],
		PendingAssets: stats["pending"],
		FailedAssets:  stats["failed"] + stats["failed_unavailable"],
		StorageUsedGB: storageGB,
		WalletsCount:  len(wallets),
		ServiceState:  serviceState,
		LastSyncAt:    lastSync,
	}

	WriteJSON(w, http.StatusOK, resp)
}

// ActivityItem represents a recent activity item
type ActivityItem struct {
	ID        uint64 `json:"id"`
	URI       string `json:"uri"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	PinnedAt  string `json:"pinned_at,omitempty"`
	NFTName   string `json:"nft_name,omitempty"`
}

// GetActivity returns recent pinned assets
// GET /api/v1/activity?limit=N
func (h *Handlers) GetActivity(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Get recently pinned assets
	var assets []db.Asset
	h.db.Where("status = ?", db.StatusPinned).
		Order("pinned_at DESC").
		Limit(limit).
		Preload("NFT").
		Find(&assets)

	resp := make([]ActivityItem, 0, len(assets))
	for _, asset := range assets {
		item := ActivityItem{
			ID:     asset.ID,
			URI:    asset.URI,
			Type:   asset.Type,
			Status: asset.Status,
		}
		if asset.PinnedAt != nil {
			item.PinnedAt = asset.PinnedAt.UTC().Format(time.RFC3339)
		}
		if asset.NFT != nil {
			item.NFTName = asset.NFT.Name
		}
		resp = append(resp, item)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// =============================================================================
// Wallet Endpoints
// =============================================================================

// WalletResponse is a single wallet in the response
type WalletResponse struct {
	Address      string  `json:"address"`
	Alias        string  `json:"alias,omitempty"`
	SyncOwned    bool    `json:"sync_owned"`
	SyncCreated  bool    `json:"sync_created"`
	LastSyncedAt *string `json:"last_synced_at,omitempty"`
	NFTCount     int     `json:"nft_count"`
}

// GetWallets returns all tracked wallets
// GET /api/v1/wallets
func (h *Handlers) GetWallets(w http.ResponseWriter, r *http.Request) {
	wallets, err := h.db.GetAllWallets()
	if err != nil {
		WriteInternalError(w, "failed to get wallets: "+err.Error())
		return
	}

	resp := make([]WalletResponse, 0, len(wallets))
	for _, wallet := range wallets {
		var nftCount int64
		h.db.Model(&db.NFT{}).Where("wallet_address = ?", wallet.Address).Count(&nftCount)

		wr := WalletResponse{
			Address:     wallet.Address,
			Alias:       wallet.Alias,
			SyncOwned:   wallet.SyncOwned,
			SyncCreated: wallet.SyncCreated,
			NFTCount:    int(nftCount),
		}
		if wallet.LastSyncedAt != nil {
			t := wallet.LastSyncedAt.UTC().Format(time.RFC3339)
			wr.LastSyncedAt = &t
		}
		resp = append(resp, wr)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// AddWalletRequest is the request body for adding a wallet
type AddWalletRequest struct {
	Address     string `json:"address"`
	Alias       string `json:"alias,omitempty"`
	SyncOwned   *bool  `json:"sync_owned,omitempty"`
	SyncCreated *bool  `json:"sync_created,omitempty"`
}

// AddWallet adds a new wallet to track
// POST /api/v1/wallets
func (h *Handlers) AddWallet(w http.ResponseWriter, r *http.Request) {
	var req AddWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "invalid JSON: "+err.Error())
		return
	}

	if req.Address == "" {
		WriteBadRequest(w, "address is required")
		return
	}

	// Check if wallet already exists
	existing, err := h.db.GetWallet(req.Address)
	if err != nil {
		WriteInternalError(w, "database error: "+err.Error())
		return
	}
	if existing != nil {
		WriteConflict(w, "wallet already exists")
		return
	}

	// Create wallet with defaults
	wallet := &db.Wallet{
		Address:     req.Address,
		Alias:       req.Alias,
		SyncOwned:   true,
		SyncCreated: true,
	}
	if req.SyncOwned != nil {
		wallet.SyncOwned = *req.SyncOwned
	}
	if req.SyncCreated != nil {
		wallet.SyncCreated = *req.SyncCreated
	}

	if err := h.db.SaveWallet(wallet); err != nil {
		WriteInternalError(w, "failed to save wallet: "+err.Error())
		return
	}

	// Trigger sync for the new wallet
	if h.service != nil {
		h.service.TriggerSync(req.Address)
	}

	resp := WalletResponse{
		Address:     wallet.Address,
		Alias:       wallet.Alias,
		SyncOwned:   wallet.SyncOwned,
		SyncCreated: wallet.SyncCreated,
		NFTCount:    0,
	}

	WriteCreated(w, resp)
}

// GetWallet returns a single wallet
// GET /api/v1/wallets/{address}
func (h *Handlers) GetWallet(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		WriteBadRequest(w, "address is required")
		return
	}

	wallet, err := h.db.GetWallet(address)
	if err != nil {
		WriteInternalError(w, "database error: "+err.Error())
		return
	}
	if wallet == nil {
		WriteNotFound(w, "wallet not found")
		return
	}

	var nftCount int64
	h.db.Model(&db.NFT{}).Where("wallet_address = ?", address).Count(&nftCount)

	resp := WalletResponse{
		Address:     wallet.Address,
		Alias:       wallet.Alias,
		SyncOwned:   wallet.SyncOwned,
		SyncCreated: wallet.SyncCreated,
		NFTCount:    int(nftCount),
	}
	if wallet.LastSyncedAt != nil {
		t := wallet.LastSyncedAt.UTC().Format(time.RFC3339)
		resp.LastSyncedAt = &t
	}

	WriteJSON(w, http.StatusOK, resp)
}

// UpdateWalletRequest is the request body for updating a wallet
type UpdateWalletRequest struct {
	Alias       *string `json:"alias,omitempty"`
	SyncOwned   *bool   `json:"sync_owned,omitempty"`
	SyncCreated *bool   `json:"sync_created,omitempty"`
}

// UpdateWallet updates a wallet's settings
// PUT /api/v1/wallets/{address}
func (h *Handlers) UpdateWallet(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		WriteBadRequest(w, "address is required")
		return
	}

	wallet, err := h.db.GetWallet(address)
	if err != nil {
		WriteInternalError(w, "database error: "+err.Error())
		return
	}
	if wallet == nil {
		WriteNotFound(w, "wallet not found")
		return
	}

	var req UpdateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "invalid JSON: "+err.Error())
		return
	}

	// Update fields if provided
	if req.Alias != nil {
		wallet.Alias = *req.Alias
	}
	if req.SyncOwned != nil {
		wallet.SyncOwned = *req.SyncOwned
	}
	if req.SyncCreated != nil {
		wallet.SyncCreated = *req.SyncCreated
	}

	if err := h.db.SaveWallet(wallet); err != nil {
		WriteInternalError(w, "failed to save wallet: "+err.Error())
		return
	}

	var nftCount int64
	h.db.Model(&db.NFT{}).Where("wallet_address = ?", address).Count(&nftCount)

	resp := WalletResponse{
		Address:     wallet.Address,
		Alias:       wallet.Alias,
		SyncOwned:   wallet.SyncOwned,
		SyncCreated: wallet.SyncCreated,
		NFTCount:    int(nftCount),
	}
	if wallet.LastSyncedAt != nil {
		t := wallet.LastSyncedAt.UTC().Format(time.RFC3339)
		resp.LastSyncedAt = &t
	}

	WriteJSON(w, http.StatusOK, resp)
}

// DeleteWallet removes a wallet
// DELETE /api/v1/wallets/{address}
// Query params: unpin=true to also unpin assets
func (h *Handlers) DeleteWallet(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		WriteBadRequest(w, "address is required")
		return
	}

	wallet, err := h.db.GetWallet(address)
	if err != nil {
		WriteInternalError(w, "database error: "+err.Error())
		return
	}
	if wallet == nil {
		WriteNotFound(w, "wallet not found")
		return
	}

	// Check if unpin was requested
	unpin := r.URL.Query().Get("unpin") == "true"

	if unpin && h.service != nil {
		// Get all assets for this wallet and unpin them
		assets, err := h.db.GetAssetsByWallet(address)
		if err != nil {
			WriteInternalError(w, "failed to get assets: "+err.Error())
			return
		}

		// Unpin each asset (best effort)
		for _, asset := range assets {
			cid := core.ExtractCIDFromURI(asset.URI)
			if cid != "" {
				_ = h.service.UnpinAsset(cid)
			}
		}

		// Delete assets from database
		if err := h.db.DeleteAssetsByWallet(address); err != nil {
			WriteInternalError(w, "failed to delete assets: "+err.Error())
			return
		}

		// Delete NFTs from database
		if err := h.db.DeleteNFTsByWallet(address); err != nil {
			WriteInternalError(w, "failed to delete NFTs: "+err.Error())
			return
		}
	}

	// Delete wallet
	if err := h.db.DeleteWallet(address); err != nil {
		WriteInternalError(w, "failed to delete wallet: "+err.Error())
		return
	}

	WriteNoContent(w)
}

// SyncWallet triggers a sync for a specific wallet
// POST /api/v1/wallets/{address}/sync
func (h *Handlers) SyncWallet(w http.ResponseWriter, r *http.Request) {
	address := chi.URLParam(r, "address")
	if address == "" {
		WriteBadRequest(w, "address is required")
		return
	}

	wallet, err := h.db.GetWallet(address)
	if err != nil {
		WriteInternalError(w, "database error: "+err.Error())
		return
	}
	if wallet == nil {
		WriteNotFound(w, "wallet not found")
		return
	}

	if h.service == nil {
		WriteServiceUnavailable(w, "backup service not available")
		return
	}

	h.service.TriggerSync(address)

	WriteAccepted(w, map[string]string{
		"message": "sync triggered",
		"wallet":  address,
	})
}

// =============================================================================
// Asset Endpoints
// =============================================================================

// AssetResponse is a single asset in the response
type AssetResponse struct {
	ID        uint64  `json:"id"`
	URI       string  `json:"uri"`
	Type      string  `json:"type"`
	MimeType  string  `json:"mime_type,omitempty"`
	Status    string  `json:"status"`
	ErrorMsg  string  `json:"error_msg,omitempty"`
	SizeBytes int64   `json:"size_bytes,omitempty"`
	PinnedAt  *string `json:"pinned_at,omitempty"`
	NFTID     uint64  `json:"nft_id"`
}

// AssetsListResponse is the paginated response for assets
type AssetsListResponse struct {
	Assets []AssetResponse `json:"assets"`
	Total  int64           `json:"total"`
	Page   int             `json:"page"`
	Limit  int             `json:"limit"`
}

// GetAssets returns paginated assets
// GET /api/v1/assets?page=N&limit=N&status=X
func (h *Handlers) GetAssets(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	// Build query
	query := h.db.Model(&db.Asset{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	var total int64
	query.Count(&total)

	// Get paginated results
	var assets []db.Asset
	query.Order("id DESC").Offset(offset).Limit(limit).Find(&assets)

	// Build response
	resp := AssetsListResponse{
		Assets: make([]AssetResponse, 0, len(assets)),
		Total:  total,
		Page:   page,
		Limit:  limit,
	}

	for _, asset := range assets {
		ar := AssetResponse{
			ID:        asset.ID,
			URI:       asset.URI,
			Type:      asset.Type,
			MimeType:  asset.MimeType,
			Status:    asset.Status,
			ErrorMsg:  asset.ErrorMsg,
			SizeBytes: asset.SizeBytes,
			NFTID:     asset.NFTID,
		}
		if asset.PinnedAt != nil {
			t := asset.PinnedAt.UTC().Format(time.RFC3339)
			ar.PinnedAt = &t
		}
		resp.Assets = append(resp.Assets, ar)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// GetFailedAssets returns all failed assets
// GET /api/v1/assets/failed
func (h *Handlers) GetFailedAssets(w http.ResponseWriter, r *http.Request) {
	var assets []db.Asset
	h.db.Where("status IN ?", []string{db.StatusFailed, db.StatusFailedUnavailable}).
		Order("id DESC").
		Find(&assets)

	resp := make([]AssetResponse, 0, len(assets))
	for _, asset := range assets {
		ar := AssetResponse{
			ID:        asset.ID,
			URI:       asset.URI,
			Type:      asset.Type,
			MimeType:  asset.MimeType,
			Status:    asset.Status,
			ErrorMsg:  asset.ErrorMsg,
			SizeBytes: asset.SizeBytes,
			NFTID:     asset.NFTID,
		}
		resp = append(resp, ar)
	}

	WriteJSON(w, http.StatusOK, resp)
}

// RetryAsset retries pinning a failed asset
// POST /api/v1/assets/{id}/retry
func (h *Handlers) RetryAsset(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		WriteBadRequest(w, "invalid asset ID")
		return
	}

	// Get the asset
	var asset db.Asset
	if err := h.db.First(&asset, id).Error; err != nil {
		WriteNotFound(w, "asset not found")
		return
	}

	// Reset status to pending
	asset.Status = db.StatusPending
	asset.ErrorMsg = ""
	asset.RetryCount = 0
	if err := h.db.Save(&asset).Error; err != nil {
		WriteInternalError(w, "failed to update asset: "+err.Error())
		return
	}

	// Trigger pin if service available
	if h.service != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			h.service.PinAsset(ctx, asset.ID)
		}()
	}

	WriteAccepted(w, map[string]interface{}{
		"message":  "retry queued",
		"asset_id": id,
	})
}

// =============================================================================
// Control Endpoints
// =============================================================================

// TriggerSync triggers a full sync operation
// POST /api/v1/sync
func (h *Handlers) TriggerSync(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		WriteServiceUnavailable(w, "backup service not available")
		return
	}

	h.service.TriggerFullSync()

	WriteAccepted(w, map[string]string{
		"message": "full sync triggered",
	})
}

// PauseService pauses the backup service
// POST /api/v1/pause
func (h *Handlers) PauseService(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		WriteServiceUnavailable(w, "backup service not available")
		return
	}

	h.service.Pause()
	WriteJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// ResumeService resumes the backup service
// POST /api/v1/resume
func (h *Handlers) ResumeService(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		WriteServiceUnavailable(w, "backup service not available")
		return
	}

	h.service.Resume()
	WriteJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// RunGC runs IPFS garbage collection
// POST /api/v1/gc
func (h *Handlers) RunGC(w http.ResponseWriter, r *http.Request) {
	if h.ipfs == nil {
		WriteServiceUnavailable(w, "IPFS node not available")
		return
	}

	// Run GC in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		h.ipfs.GarbageCollect(ctx)
	}()

	WriteAccepted(w, map[string]string{
		"message": "garbage collection started",
	})
}

// DiscoverServers scans for Porcupin servers on the local network via mDNS
// GET /api/v1/discover
func (h *Handlers) DiscoverServers(w http.ResponseWriter, r *http.Request) {
	// Parse timeout from query params (default 5 seconds)
	timeoutStr := r.URL.Query().Get("timeout")
	timeout := 5 * time.Second
	if timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 && t <= 30 {
			timeout = time.Duration(t) * time.Second
		}
	}

	servers, err := DiscoverServers(r.Context(), timeout)
	if err != nil {
		WriteInternalError(w, "mDNS discovery failed: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, servers)
}
