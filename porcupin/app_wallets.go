package main

import (
	"fmt"
	"log/slog"
	"regexp"

	"porcupin/backend/core"
	"porcupin/backend/db"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// tezosAddressPattern validates Tezos wallet addresses (tz1, tz2, tz3, KT1)
var tezosAddressPattern = regexp.MustCompile(`^(tz[1-3]|KT1)[a-zA-Z0-9]{33}$`)

// GetWallets retrieves all tracked wallets
func (a *App) GetWallets() ([]db.Wallet, error) {
	var wallets []db.Wallet
	if err := a.database.Find(&wallets).Error; err != nil {
		return nil, err
	}
	return wallets, nil
}

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
	slog.Info("Deleting wallet with unpin", "address", address)

	wailsRuntime.EventsEmit(a.ctx, "wallet:delete:start", map[string]string{
		"address": address,
	})

	err := a.backupService.DeleteWalletFull(address, func(phase string, total, current int) {
		wailsRuntime.EventsEmit(a.ctx, "wallet:delete:progress", map[string]interface{}{
			"address": address,
			"phase":   phase,
			"total":   total,
			"current": current,
		})
	})

	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "wallet:delete:error", map[string]interface{}{
			"address": address,
			"error":   err.Error(),
		})
		return err
	}

	wailsRuntime.EventsEmit(a.ctx, "wallet:delete:complete", map[string]interface{}{
		"address": address,
	})

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
