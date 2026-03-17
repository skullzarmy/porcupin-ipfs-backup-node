package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"porcupin/backend/config"
	"porcupin/backend/core"
	"porcupin/backend/db"
	"porcupin/backend/indexer"
	"porcupin/backend/ipfs"
	"porcupin/backend/logging"
	"porcupin/backend/updater"
	"porcupin/backend/version"

	"github.com/glebarez/sqlite"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"
)

// App struct
type App struct {
	ctx           context.Context
	config        *config.Config
	database      *db.Database
	ipfsNode      *ipfs.Node
	indexer       *indexer.Indexer
	backupService *core.BackupService
	updater       *updater.Manager
	logRing       *logging.RingHandler
	logFile       *os.File
}

// NewApp creates a new App application struct
func NewApp(logRing *logging.RingHandler, logFile *os.File) *App {
	return &App{logRing: logRing, logFile: logFile}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("Porcupin starting up...")

	// Setup data directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home dir: %v", err)
	}
	dataDir := filepath.Join(homeDir, ".porcupin")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data dir: %v", err)
	}

	// Load configuration
	configPath := filepath.Join(dataDir, "config.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
		// Ensure IPFS path is absolute if default
		if cfg.IPFS.RepoPath == "~/.porcupin/ipfs" {
			cfg.IPFS.RepoPath = filepath.Join(dataDir, "ipfs")
		}
	}
	a.config = cfg

	// Initialize database
	dbPath := filepath.Join(dataDir, "porcupin.db")
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err := db.InitDB(gormDB); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	a.database = db.NewDatabase(gormDB)
	log.Println("Database initialized")

	// Initialize IPFS node
	// Ensure repo path is absolute
	repoPath := cfg.IPFS.RepoPath
	if strings.HasPrefix(repoPath, "~/") {
		repoPath = filepath.Join(homeDir, repoPath[2:])
	}
	
	ipfsNode, err := ipfs.NewNode(repoPath, cfg.IPFS.SwarmPort)
	if err != nil {
		log.Fatalf("Failed to create IPFS node: %v", err)
	}

	if err := ipfsNode.Start(ctx); err != nil {
		log.Printf("Failed to start IPFS node: %v", err)
		wailsRuntime.MessageDialog(ctx, wailsRuntime.MessageDialogOptions{
			Type:  wailsRuntime.ErrorDialog,
			Title: "Startup Failed",
			Message: "Could not start the IPFS node.\n\nA previous instance may still be " +
				"shutting down. Please wait 30 seconds and try again.\n\n" +
				"If this persists, restart your computer.\n\nError: " + err.Error(),
		})
		os.Exit(1)
	}

	a.ipfsNode = ipfsNode
	log.Println("IPFS node started")

	// Initialize indexer
	a.indexer = indexer.NewIndexer(cfg.TZKT.BaseURL)
	log.Println("Indexer initialized")

	// Initialize backup service (handles automatic syncing)
	a.backupService = core.NewBackupService(ipfsNode, a.indexer, a.database, cfg)
	log.Println("Backup service initialized")
	
	// Initialize updater
	updaterMgr, err := updater.NewManager(version.Version)
	if err != nil {
		log.Printf("Failed to initialize updater: %v", err)
	} else {
		a.updater = updaterMgr
		log.Println("Updater initialized (current version: " + version.Version + ")")

		// Check for updates in background
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("goroutine panic recovered", "goroutine", "update-checker", "panic", r)
				}
			}()
			// Wait a bit for UI to be ready
			time.Sleep(5 * time.Second)
			if info, err := a.updater.CheckForUpdates(ctx); err == nil && info.Available {
				wailsRuntime.EventsEmit(ctx, "update:available", info)
			}
		}()
	}
	
	// Initialize disk usage in background (don't block startup)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered", "goroutine", "disk-usage-init", "panic", r)
			}
		}()
		a.backupService.GetManager().MarkDiskUsageDirty()
		a.backupService.GetManager().UpdateDiskUsage()
	}()
	
	// Start the automatic backup service
	a.backupService.Start(ctx)
	a.backupService.SetWailsCtx(ctx)
	log.Println("Backup service started - auto-syncing enabled")

	log.Println("Porcupin startup complete!")
}

// shutdown is called during application termination
func (a *App) shutdown(ctx context.Context) {
	// Hard deadline: guarantee the process exits even if IPFS close hangs or
	// WebKitGTK lingers. ShutdownTimeout is 30s for IPFS; 35s gives it a fair
	// chance before we force exit and let the OS reclaim all ports.
	go func() {
		time.Sleep(35 * time.Second)
		log.Println("Forced process exit — shutdown exceeded 35 seconds")
		os.Exit(0)
	}()

	log.Println("Porcupin shutting down...")

	if a.backupService != nil {
		a.backupService.Stop()
	}

	if a.ipfsNode != nil {
		if err := a.ipfsNode.Stop(); err != nil {
			log.Printf("Error stopping IPFS node: %v", err)
		}
	}

	log.Println("Porcupin shutdown complete")

	if a.logFile != nil {
		a.logFile.Close()
	}
}

// domReady is called after front-end resources have been loaded
func (a *App) domReady(ctx context.Context) {
	// Show and focus the window
	wailsRuntime.WindowShow(ctx)
	wailsRuntime.WindowUnminimise(ctx)
	wailsRuntime.Show(ctx)
}

// beforeClose is called when the application is about to quit
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

// GetStatus returns the current status of the application
func (a *App) GetStatus() map[string]interface{} {
	stats, _ := a.database.GetAssetStats()
	wallets, _ := a.GetWallets()
	
	return map[string]interface{}{
		"running":       true,
		"wallets_count": len(wallets),
		"pinned_count":  stats["pinned"],
		"pending_count": stats["pending"],
		"failed_count":  stats["failed"],
	}
}

// GetVersion returns the current version of Porcupin
func (a *App) GetVersion() string {
	return version.Version
}

// GetIPFSHealth returns current peer connectivity status from the embedded IPFS node.
func (a *App) GetIPFSHealth() ipfs.NodeHealthResult {
	if a.ipfsNode == nil {
		return ipfs.NodeHealthResult{IsOnline: false, Message: "Node not initialized"}
	}
	return a.ipfsNode.Health(a.ctx)
}
