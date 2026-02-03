package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"porcupin/backend/db"
	"porcupin/backend/ipfs"
)

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
