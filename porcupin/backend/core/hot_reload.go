package core

import (
	"log"
)

// checkForNewWallets checks the DB for any wallets that aren't being watched yet
func (s *BackupService) checkForNewWallets() {
	wallets, err := s.db.GetAllWallets()
	if err != nil {
		log.Printf("Hot Reload: Failed to get wallets: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, wallet := range wallets {
		if !s.watchedWallets[wallet.Address] {
			log.Printf("Hot Reload: Found new wallet %s, starting watcher...", wallet.Address)
			
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
