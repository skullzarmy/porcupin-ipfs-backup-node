package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"porcupin/backend/cli"
	"porcupin/backend/config"
	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
	"porcupin/backend/version"
)

func main() {

	// Parse command line flags
	configPath := flag.String("config", "", "Path to config file (default: ~/.porcupin/config.yaml)")
	dataDir := flag.String("data", "", "Data directory (default: ~/.porcupin)")
	addWallet := flag.String("add-wallet", "", "Add a wallet address and exit")
	listWallets := flag.Bool("list-wallets", false, "List all tracked wallets and exit")
	removeWallet := flag.String("remove-wallet", "", "Remove a wallet address and exit")
	unpinWallet := flag.String("unpin-wallet", "", "Unpin all assets for a wallet and exit")
	deleteWallet := flag.String("delete-wallet", "", "Remove wallet and unpin all its assets, then exit")
	runGC := flag.Bool("gc", false, "Run IPFS garbage collection and exit")
	showStats := flag.Bool("stats", false, "Show current stats and exit")
	showVersion := flag.Bool("version", false, "Show version and exit")
	showVersionShort := flag.Bool("v", false, "Show version and exit")
	showAbout := flag.Bool("about", false, "Show about information and exit")
	retryPending := flag.Bool("retry-pending", false, "Process all pending assets and exit")
	flag.Parse()

	if *showVersion || *showVersionShort {
		cli.PrintBannerWithVersion(version.Version)
		return
	}

	if *showAbout {
		cli.PrintAbout(version.Version)
		return
	}

	// Determine data directory
	var dataPath string
	if *dataDir != "" {
		dataPath = *dataDir
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}
		dataPath = filepath.Join(homeDir, ".porcupin")
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Load configuration
	var cfgPath string
	if *configPath != "" {
		cfgPath = *configPath
	} else {
		cfgPath = filepath.Join(dataPath, "config.yaml")
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Printf("No config file found, using defaults")
		cfg = config.DefaultConfig()
	}

	// Initialize database
	dbPath := filepath.Join(dataPath, "porcupin.db")
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	if err := db.InitDB(gormDB); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	database := db.NewDatabase(gormDB)

	// Handle one-off commands
	if *addWallet != "" {
		wallet := &db.Wallet{Address: *addWallet, Alias: ""}
		if err := database.SaveWallet(wallet); err != nil {
			log.Fatalf("Failed to add wallet: %v", err)
		}
		fmt.Printf("Added wallet: %s\n", *addWallet)
		return
	}

	if *removeWallet != "" {
		if err := database.DeleteWallet(*removeWallet); err != nil {
			log.Fatalf("Failed to remove wallet: %v", err)
		}
		fmt.Printf("Removed wallet: %s (assets still pinned, use --unpin-wallet to unpin)\n", *removeWallet)
		return
	}

	// Commands that require IPFS: unpin-wallet, delete-wallet, gc
	if *unpinWallet != "" || *deleteWallet != "" || *runGC {
		// Start IPFS node
		ipfsRepoPath := filepath.Join(dataPath, "ipfs")
		ipfsNode, err := ipfs.NewNode(ipfsRepoPath)
		if err != nil {
			log.Fatalf("Failed to create IPFS node: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := ipfsNode.Start(ctx); err != nil {
			log.Fatalf("Failed to start IPFS node: %v", err)
		}
		defer ipfsNode.Stop()

		if *unpinWallet != "" {
			assets, err := database.GetAssetsByWallet(*unpinWallet)
			if err != nil {
				log.Fatalf("Failed to get assets: %v", err)
			}
			if len(assets) == 0 {
				fmt.Printf("No assets found for wallet: %s\n", *unpinWallet)
				return
			}
			fmt.Printf("Unpinning %d assets for wallet %s...\n", len(assets), *unpinWallet)
			unpinned := 0
			for _, asset := range assets {
				cid := core.ExtractCIDFromURI(asset.URI)
				if cid == "" {
					continue
				}
				if err := ipfsNode.Unpin(ctx, cid); err != nil {
					log.Printf("Warning: failed to unpin %s: %v", cid, err)
				} else {
					unpinned++
				}
			}
			fmt.Printf("Unpinned %d/%d assets. Run --gc to reclaim disk space.\n", unpinned, len(assets))
			return
		}

		if *deleteWallet != "" {
			assets, err := database.GetAssetsByWallet(*deleteWallet)
			if err != nil {
				log.Fatalf("Failed to get assets: %v", err)
			}
			fmt.Printf("Deleting wallet %s: unpinning %d assets...\n", *deleteWallet, len(assets))
			for _, asset := range assets {
				cid := core.ExtractCIDFromURI(asset.URI)
				if cid == "" {
					continue
				}
				if err := ipfsNode.Unpin(ctx, cid); err != nil {
					log.Printf("Warning: failed to unpin %s: %v", cid, err)
				}
			}
			// Delete from database
			if err := database.DeleteAssetsByWallet(*deleteWallet); err != nil {
				log.Printf("Warning: failed to delete assets from DB: %v", err)
			}
			if err := database.DeleteNFTsByWallet(*deleteWallet); err != nil {
				log.Printf("Warning: failed to delete NFTs from DB: %v", err)
			}
			if err := database.DeleteWallet(*deleteWallet); err != nil {
				log.Fatalf("Failed to delete wallet: %v", err)
			}
			fmt.Printf("Deleted wallet %s and unpinned assets. Run --gc to reclaim disk space.\n", *deleteWallet)
			return
		}

		if *runGC {
			fmt.Println("Running IPFS garbage collection...")
			if err := ipfsNode.GarbageCollect(ctx); err != nil {
				log.Fatalf("Garbage collection failed: %v", err)
			}
			fmt.Println("Garbage collection complete.")
			return
		}
	}

	if *listWallets {
		wallets, err := database.GetAllWallets()
		if err != nil {
			log.Fatalf("Failed to get wallets: %v", err)
		}
		if len(wallets) == 0 {
			fmt.Println("No wallets configured")
		} else {
			fmt.Println("Tracked wallets:")
			for _, w := range wallets {
				alias := w.Alias
				if alias == "" {
					alias = "(no alias)"
				}
				fmt.Printf("  %s - %s\n", w.Address, alias)
			}
		}
		return
	}

	if *showStats {
		stats, err := database.GetAssetStats()
		if err != nil {
			log.Fatalf("Failed to get stats: %v", err)
		}
		totalAssets := stats["pending"] + stats["pinned"] + stats["failed"] + stats["failed_unavailable"]
		
		// Get actual disk usage from IPFS repo directory
		ipfsRepoPath := filepath.Join(dataPath, "ipfs")
		storageBytes, err := core.GetDiskUsageBytes(ipfsRepoPath)
		if err != nil {
			log.Printf("Warning: could not get disk usage: %v", err)
			storageBytes = 0
		}
		
		cli.PrintStats(
			stats["nft_count"],
			totalAssets,
			stats["pinned"],
			stats["pending"],
			stats["failed"]+stats["failed_unavailable"],
			float64(storageBytes)/(1024*1024*1024),
		)
		return
	}

	// Handle --retry-pending (requires IPFS)
	if *retryPending {
		// Check if there are pending assets first
		stats, err := database.GetAssetStats()
		if err != nil {
			log.Fatalf("Failed to get stats: %v", err)
		}
		pendingCount := stats["pending"]
		if pendingCount == 0 {
			fmt.Println("No pending assets to process")
			return
		}

		fmt.Printf("Found %d pending assets, starting IPFS node...\n", pendingCount)

		ipfsRepoPath := filepath.Join(dataPath, "ipfs")
		ipfsNode, err := ipfs.NewNode(ipfsRepoPath)
		if err != nil {
			log.Fatalf("Failed to create IPFS node: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := ipfsNode.Start(ctx); err != nil {
			log.Fatalf("Failed to start IPFS node: %v", err)
		}
		defer ipfsNode.Stop()

		fmt.Println("IPFS node started, processing pending assets...")

		// Create a minimal backup manager just for pinning
		idx := indexer.NewIndexer(cfg.TZKT.BaseURL)
		manager := core.NewBackupManager(ipfsNode, idx, database, cfg)

		processed, pinned, failed := manager.ProcessPendingAssets(ctx, 0) // 0 = no limit
		fmt.Printf("Processed %d assets: %d pinned, %d failed\n", processed, pinned, failed)
		return
	}

	// Start IPFS node
	fmt.Println("🦔 Porcupin Headless Server")
	fmt.Println("Starting IPFS node...")

	ipfsRepoPath := filepath.Join(dataPath, "ipfs")
	ipfsNode, err := ipfs.NewNode(ipfsRepoPath)
	if err != nil {
		log.Fatalf("Failed to create IPFS node: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ipfsNode.Start(ctx); err != nil {
		log.Fatalf("Failed to start IPFS node: %v", err)
	}
	defer ipfsNode.Stop()

	fmt.Println("IPFS node started")

	// Initialize indexer
	idx := indexer.NewIndexer(cfg.TZKT.BaseURL)

	// Create and start backup service
	service := core.NewBackupService(ipfsNode, idx, database, cfg)

	service.Start(ctx)
	fmt.Println("Backup service started. Monitoring wallets...")

	// Print initial wallet count
	wallets, _ := database.GetAllWallets()
	fmt.Printf("Tracking %d wallet(s)\n", len(wallets))

	// Status ticker
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigCh:
			fmt.Println("\nShutting down...")
			service.Stop()
			return
		case <-statusTicker.C:
			status := service.GetStatus()
			stats, _ := database.GetAssetStats()
			log.Printf("[%s] Pinned: %d/%d, Failed: %d, Pending retries: %d",
				status.State,
				stats["pinned_assets"],
				stats["total_assets"],
				stats["failed_assets"],
				status.PendingRetries,
			)
		}
	}
}
