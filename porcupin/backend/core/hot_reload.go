package core

import (
	"log/slog"
)

// checkForNewWallets checks the DB for any wallets that aren't being watched yet
func (s *BackupService) checkForNewWallets() {
	wallets, err := s.db.GetAllWallets()
	if err != nil {
		slog.Error("Hot Reload: failed to get wallets", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, wallet := range wallets {
		if !s.watchedWallets[wallet.Address] {
			slog.Info("Hot Reload: found new wallet, starting watcher", "address", wallet.Address)
			
			// Mark as watched
			s.watchedWallets[wallet.Address] = true
			
			// Start watcher
			go s.watchWallet(wallet.Address)
			
			// Trigger immediate sync
			select {
			case s.triggerCh <- wallet.Address:
			default:
			}
		}
	}
}
