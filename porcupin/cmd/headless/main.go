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

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"porcupin/backend/config"
	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to config file (default: ~/.porcupin/config.yaml)")
	dataDir := flag.String("data", "", "Data directory (default: ~/.porcupin)")
	addWallet := flag.String("add-wallet", "", "Add a wallet address and exit")
	listWallets := flag.Bool("list-wallets", false, "List all tracked wallets and exit")
	removeWallet := flag.String("remove-wallet", "", "Remove a wallet address and exit")
	showStats := flag.Bool("stats", false, "Show current stats and exit")
	flag.Parse()

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
		fmt.Printf("Removed wallet: %s\n", *removeWallet)
		return
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
		fmt.Println("Porcupin Stats:")
		fmt.Printf("  Total NFTs:     %d\n", stats["total_nfts"])
		fmt.Printf("  Total Assets:   %d\n", stats["total_assets"])
		fmt.Printf("  Pinned:         %d\n", stats["pinned_assets"])
		fmt.Printf("  Pending:        %d\n", stats["pending_assets"])
		fmt.Printf("  Failed:         %d\n", stats["failed_assets"])
		fmt.Printf("  Storage Used:   %.2f GB\n", float64(stats["total_size_bytes"])/(1024*1024*1024))
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
