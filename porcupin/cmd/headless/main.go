package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"porcupin/backend/api"
	"porcupin/backend/cli"
	"porcupin/backend/config"
	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
	"porcupin/backend/updater"
	"porcupin/backend/version"
)

func main() {

	// Parse command line flags
	configPath := flag.String("config", "", "Path to config file (default: ~/.porcupin/config.yaml)")
	dataDir := flag.String("data", "", "Data directory (default: ~/.porcupin)")
	addWallet := flag.String("add-wallet", "", "Add a wallet address and exit")
	walletAlias := flag.String("alias", "", "Alias for wallet (use with --add-wallet or --rename-wallet)")
	renameWallet := flag.String("rename-wallet", "", "Rename a wallet (set alias), use with --alias")
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
	updateCheck := flag.Bool("update", false, "Check for and install updates")

	// API server flags
	serveAPI := flag.Bool("serve", false, "Start API server for remote access")
	apiPort := flag.Int("api-port", 8085, "API server port (use with --serve)")
	apiBind := flag.String("api-bind", "0.0.0.0", "API server bind address (use with --serve)")
	apiToken := flag.String("api-token", "", "Set API token (WARNING: visible in ps, prefer env var)")
	allowPublic := flag.Bool("allow-public", false, "Allow public IP connections (use with --serve)")
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate file (use with --serve)")
	tlsKey := flag.String("tls-key", "", "Path to TLS private key file (use with --serve)")
	regenerateToken := flag.Bool("regenerate-token", false, "Regenerate API token and exit")

	// IPFS flags
	ipfsPort := flag.Int("ipfs-port", 0, "IPFS swarm port (default 4001, 0 = use config)")

	flag.Parse()

	// Warn if --api-token flag is used (visible in ps)
	if *apiToken != "" {
		slog.Warn("--api-token flag is visible in process list, consider using PORCUPIN_API_TOKEN env var instead")
	}

	if *showVersion || *showVersionShort {
		cli.PrintBannerWithVersion(version.Version)
		return
	}

	if *showAbout {
		cli.PrintAbout(version.Version)
		return
	}

	if *updateCheck {
		updateMgr, err := updater.NewServerManager(version.Version)
		if err != nil {
			slog.Error("Failed to initialize updater", "error", err)
			os.Exit(1)
		}
		fmt.Println("Checking for updates...")
		// Use timeout for update check
		ctxCheck, cancelCheck := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCheck()

		info, err := updateMgr.CheckForUpdates(ctxCheck)
		if err != nil {
			slog.Error("Failed to check for updates", "error", err)
			os.Exit(1)
		}
		if !info.Available {
			fmt.Printf("Porcupin is up to date (version %s)\n", version.Version)
			return
		}

		fmt.Printf("New version available: %s\n", info.Version)
		fmt.Printf("Release notes:\n%s\n", info.ReleaseNotes)
		fmt.Print("Installing update... ")
		
		// Use longer timeout for download and install
		ctxInstall, cancelInstall := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancelInstall()
		
		if err := updateMgr.InstallLatest(ctxInstall); err != nil {
			fmt.Printf("Failed\n")
			slog.Error("Failed to install update", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Success!\n")
		fmt.Println("Please restart the application.")
		return
	}

	// Determine data directory
	var dataPath string
	if *dataDir != "" {
		dataPath = *dataDir
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			slog.Error("Failed to get home directory", "error", err)
			os.Exit(1)
		}
		dataPath = filepath.Join(homeDir, ".porcupin")
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		slog.Error("Failed to create data directory", "error", err)
		os.Exit(1)
	}

	// Handle token management commands (before other initialization)
	if *regenerateToken {
		token, err := api.RegenerateToken(dataPath)
		if err != nil {
			slog.Error("Failed to regenerate token", "error", err)
			os.Exit(1)
		}
		fmt.Println("New API token generated:")
		fmt.Println()
		fmt.Printf("  %s\n", token)
		fmt.Println()
		fmt.Println("⚠️  Save this token securely - it will not be shown again!")
		fmt.Println("   Token hash stored at:", filepath.Join(dataPath, api.TokenFileName))
		return
	}

	// Load configuration
	var cfgPath string
	if *configPath != "" {
		cfgPath = *configPath
	} else {
		cfgPath = filepath.Join(dataPath, "config.yaml")
	}

	// Ensure config file exists on disk (auto-generate defaults if missing)
	created, ensureErr := config.EnsureConfigFile(cfgPath)
	if ensureErr != nil {
		slog.Warn("Could not create config file", "error", ensureErr)
	} else if created {
		slog.Info("Created default config", "path", cfgPath)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		slog.Info("No config file found, using defaults")
		cfg = config.DefaultConfig()
	}

	// Handle positional subcommands (before DB/IPFS initialization)
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "version":
			cli.PrintBannerWithVersion(version.Version)
			return
		case "about":
			cli.PrintAbout(version.Version)
			return
		case "settings":
			handleSettings(args[1:], cfg, cfgPath)
			return
		}
	}

	// Apply CLI overrides to config
	if *ipfsPort > 0 {
		cfg.IPFS.SwarmPort = *ipfsPort
		slog.Info("Using CLI-specified IPFS swarm port", "port", *ipfsPort)
	}

	// Initialize database
	dbPath := filepath.Join(dataPath, "porcupin.db")
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn), // Suppress "record not found" info logs
	})
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	if err := db.InitDB(gormDB); err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	database := db.NewDatabase(gormDB)

	// Handle one-off commands
	if *addWallet != "" {
		wallet := &db.Wallet{Address: *addWallet, Alias: *walletAlias}
		if err := database.SaveWallet(wallet); err != nil {
			slog.Error("Failed to add wallet", "error", err)
			os.Exit(1)
		}
		if *walletAlias != "" {
			fmt.Printf("Added wallet: %s (%s)\n", *walletAlias, *addWallet)
		} else {
			fmt.Printf("Added wallet: %s\n", *addWallet)
		}
		return
	}

	if *renameWallet != "" {
		if err := database.Model(&db.Wallet{}).Where("address = ?", *renameWallet).Update("alias", *walletAlias).Error; err != nil {
			slog.Error("Failed to rename wallet", "error", err)
			os.Exit(1)
		}
		if *walletAlias != "" {
			fmt.Printf("Renamed wallet %s to: %s\n", *renameWallet, *walletAlias)
		} else {
			fmt.Printf("Cleared alias for wallet: %s\n", *renameWallet)
		}
		return
	}

	if *removeWallet != "" {
		if err := database.DeleteWallet(*removeWallet); err != nil {
			slog.Error("Failed to remove wallet", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Removed wallet: %s (assets still pinned, use --unpin-wallet to unpin)\n", *removeWallet)
		return
	}

	// Commands that require IPFS: unpin-wallet, delete-wallet, gc
	if *unpinWallet != "" || *deleteWallet != "" || *runGC {
		// Start IPFS node
		ipfsRepoPath := filepath.Join(dataPath, "ipfs")
		ipfsNode, err := ipfs.NewNode(ipfsRepoPath, cfg.IPFS.SwarmPort)
		if err != nil {
			slog.Error("Failed to create IPFS node", "error", err)
			os.Exit(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := ipfsNode.Start(ctx); err != nil {
			slog.Error("Failed to start IPFS node", "error", err)
			os.Exit(1)
		}
		defer ipfsNode.Stop()

		if *unpinWallet != "" {
			assets, err := database.GetAssetsByWallet(*unpinWallet)
			if err != nil {
				slog.Error("Failed to get assets", "error", err)
				os.Exit(1)
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
					slog.Warn("Failed to unpin", "cid", cid, "error", err)
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
				slog.Error("Failed to get assets", "error", err)
				os.Exit(1)
			}
			fmt.Printf("Deleting wallet %s: unpinning %d assets...\n", *deleteWallet, len(assets))
			for _, asset := range assets {
				cid := core.ExtractCIDFromURI(asset.URI)
				if cid == "" {
					continue
				}
				if err := ipfsNode.Unpin(ctx, cid); err != nil {
					slog.Warn("Failed to unpin", "cid", cid, "error", err)
				}
			}
			// Delete from database
			if err := database.DeleteAssetsByWallet(*deleteWallet); err != nil {
				slog.Warn("Failed to delete assets from DB", "error", err)
			}
			if err := database.DeleteNFTsByWallet(*deleteWallet); err != nil {
				slog.Warn("Failed to delete NFTs from DB", "error", err)
			}
			if err := database.DeleteWallet(*deleteWallet); err != nil {
				slog.Error("Failed to delete wallet", "error", err)
				os.Exit(1)
			}
			fmt.Printf("Deleted wallet %s and unpinned assets. Run --gc to reclaim disk space.\n", *deleteWallet)
			return
		}

		if *runGC {
			fmt.Println("Running IPFS garbage collection...")
			if err := ipfsNode.GarbageCollect(ctx); err != nil {
				slog.Error("Garbage collection failed", "error", err)
				os.Exit(1)
			}
			fmt.Println("Garbage collection complete.")
			return
		}
	}

	if *listWallets {
		wallets, err := database.GetAllWallets()
		if err != nil {
			slog.Error("Failed to get wallets", "error", err)
			os.Exit(1)
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
			slog.Error("Failed to get stats", "error", err)
			os.Exit(1)
		}
		totalAssets := stats["pending"] + stats["pinned"] + stats["failed"] + stats["failed_unavailable"]
		
		// Get actual disk usage from IPFS repo directory
		ipfsRepoPath := resolveRepoPath(cfg, dataPath)
		storageBytes, err := core.GetDiskUsageBytes(ipfsRepoPath)
		if err != nil {
			slog.Warn("Could not get disk usage", "error", err)
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
			slog.Error("Failed to get stats", "error", err)
			os.Exit(1)
		}
		pendingCount := stats["pending"]
		if pendingCount == 0 {
			fmt.Println("No pending assets to process")
			return
		}

		fmt.Printf("Found %d pending assets, starting IPFS node...\n", pendingCount)

		ipfsRepoPath := resolveRepoPath(cfg, dataPath)
		ipfsNode, err := ipfs.NewNode(ipfsRepoPath, cfg.IPFS.SwarmPort)
		if err != nil {
			slog.Error("Failed to create IPFS node", "error", err)
			os.Exit(1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := ipfsNode.Start(ctx); err != nil {
			slog.Error("Failed to start IPFS node", "error", err)
			os.Exit(1)
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

	ipfsRepoPath := resolveRepoPath(cfg, dataPath)
	ipfsNode, err := ipfs.NewNode(ipfsRepoPath, cfg.IPFS.SwarmPort)
	if err != nil {
		slog.Error("Failed to create IPFS node", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ipfsNode.Start(ctx); err != nil {
		slog.Error("Failed to start IPFS node", "error", err)
		os.Exit(1)
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

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start API server if requested
	if *serveAPI {
		var plainToken string  // Used for env var or flag (direct comparison)
		var tokenHash string   // Used for file-based auth (bcrypt comparison)
		var isNew bool

		if *apiToken != "" {
			// Token from flag (already warned above)
			plainToken = *apiToken
			fmt.Println("Using API token from --api-token flag")
		} else if envToken := api.GetTokenFromEnv(); envToken != "" {
			// Token from environment variable
			if !api.ValidateTokenFormat(envToken) {
				slog.Error("PORCUPIN_API_TOKEN has invalid format")
				os.Exit(1)
			}
			plainToken = envToken
			fmt.Println("Using API token from PORCUPIN_API_TOKEN environment variable")
		} else {
			// Check for existing token hash in file, or create new token
			var err error
			tokenHash, err = api.GetTokenHashFromFile(dataPath)
			if err != nil {
				slog.Error("Failed to read token file", "error", err)
				os.Exit(1)
			}

			if tokenHash == "" {
				// No token exists - generate new one
				var newToken string
				newToken, isNew, err = api.GetOrCreateToken(dataPath)
				if err != nil {
					slog.Error("Failed to create API token", "error", err)
					os.Exit(1)
				}

				if isNew {
					fmt.Println()
					fmt.Println("═══════════════════════════════════════════════════════════════")
					fmt.Println("  NEW API TOKEN GENERATED - SAVE THIS NOW!")
					fmt.Println("═══════════════════════════════════════════════════════════════")
					fmt.Println()
					fmt.Printf("  %s\n", newToken)
					fmt.Println()
					fmt.Println("  ⚠️  This token will NOT be displayed again!")
					fmt.Println("  Token hash stored at:", filepath.Join(dataPath, api.TokenFileName))
					fmt.Println("═══════════════════════════════════════════════════════════════")
					fmt.Println()

					// Re-read the hash we just created
					tokenHash, err = api.GetTokenHashFromFile(dataPath)
					if err != nil {
						slog.Error("Failed to read token hash", "error", err)
						os.Exit(1)
					}
				}
			} else {
				fmt.Println("Using API token from file (hash-based authentication)")
			}
		}

		// Create API server config
		serverCfg := api.ServerConfig{
			Port:            *apiPort,
			BindAddress:     *apiBind,
			Token:           plainToken,
			TokenHash:       tokenHash,
			AllowPublic:     *allowPublic,
			DataDir:         dataPath,
			Version:         version.Version,
			PerIPRateLimit:  10,
			GlobalRateLimit: 100,
			TLSCert:         *tlsCert,
			TLSKey:          *tlsKey,
		}

		// Create and start API server in a goroutine
		apiServer := api.NewServer(serverCfg, database, service)
		apiServer.SetIPFS(ipfsNode)
		go func() {
			if err := apiServer.Start(ctx); err != nil {
				slog.Error("API server error", "error", err)
			}
		}()
	}

	// Status ticker
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("\nShutting down...")
			service.Stop()

			// Explicitly close database to ensure WAL checkpoint
			sqlDB, err := gormDB.DB()
			if err == nil {
				slog.Info("Closing database connection...")
				if err := sqlDB.Close(); err != nil {
					slog.Error("Error closing database", "error", err)
				} else {
					slog.Info("Database connection closed (checkpointed)")
				}
			}
			return
		case <-statusTicker.C:
			status := service.GetStatus()
			stats, _ := database.GetAssetStats()
			slog.Info("Status update",
				"state", status.State,
				"pinned", stats["pinned_assets"],
				"total", stats["total_assets"],
				"failed", stats["failed_assets"],
				"pending_retries", status.PendingRetries,
			)
		}
	}
}

// handleSettings processes the 'porcupin settings' subcommand
func handleSettings(args []string, cfg *config.Config, cfgPath string) {
	if len(args) == 0 {
		printSettingsUsage()
		return
	}

	subcmd := args[0]

	switch subcmd {
	case "list":
		items := config.ListAll(cfg)
		useColor := cli.IsTTY()
		if useColor {
			fmt.Println()
			fmt.Printf("  %s%sPorcupin Settings%s\n", cli.Bold, cli.White, cli.Reset)
			fmt.Println()
		}
		currentSection := ""
		for _, item := range items {
			// Extract section name for grouping
			parts := splitFirst(item.Key, ".")
			if parts[0] != currentSection {
				currentSection = parts[0]
				if useColor {
					fmt.Printf("  %s[%s]%s\n", cli.Dim, currentSection, cli.Reset)
				} else {
					fmt.Printf("  [%s]\n", currentSection)
				}
			}
			if useColor {
				fmt.Printf("    %s%s%s = %s%s%s\n", cli.Cyan, item.Key, cli.Reset, cli.Bold, item.Value, cli.Reset)
			} else {
				fmt.Printf("    %s = %s\n", item.Key, item.Value)
			}
		}
		if useColor {
			fmt.Println()
		}

	case "location":
		fmt.Println(cfgPath)

	default:
		// It's a key — either get or set
		key := subcmd

		if len(args) == 1 {
			// GET
			val, err := config.GetByDotNotation(cfg, key)
			if err != nil {
				slog.Error("Settings error", "error", err)
				os.Exit(1)
			}
			fmt.Println(val)
		} else {
			// SET
			value := args[1]

			// Show old value
			oldVal, _ := config.GetByDotNotation(cfg, key)

			if err := config.SetByDotNotation(cfg, key, value); err != nil {
				slog.Error("Settings error", "error", err)
				os.Exit(1)
			}

			if err := cfg.SaveConfig(cfgPath); err != nil {
				slog.Error("Failed to save config", "error", err)
				os.Exit(1)
			}

			newVal, _ := config.GetByDotNotation(cfg, key)
			if cli.IsTTY() {
				fmt.Printf("%s%s%s: %s → %s%s%s\n", cli.Cyan, key, cli.Reset, oldVal, cli.Bold, newVal, cli.Reset)
			} else {
				fmt.Printf("%s: %s → %s\n", key, oldVal, newVal)
			}
		}
	}
}

// printSettingsUsage prints help for the settings subcommand
func printSettingsUsage() {
	fmt.Println("Usage: porcupin settings <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  list                          List all settings")
	fmt.Println("  location                      Show config file path")
	fmt.Println("  <key>                         Get a setting value")
	fmt.Println("  <key> <value>                 Set a setting value")
	fmt.Println()
	fmt.Println("Keys use dot notation (e.g. backup.max_concurrency)")
	fmt.Println("Dashes and underscores are interchangeable.")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  porcupin settings list")
	fmt.Println("  porcupin settings backup.max_concurrency")
	fmt.Println("  porcupin settings backup.max-concurrency 2")
	fmt.Println("  porcupin settings ipfs.pin_timeout 5m")
}

// splitFirst splits a string on the first occurrence of sep
func splitFirst(s, sep string) [2]string {
	idx := len(s)
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			idx = i
			break
		}
	}
	if idx == len(s) {
		return [2]string{s, ""}
	}
	return [2]string{s[:idx], s[idx+1:]}
}
