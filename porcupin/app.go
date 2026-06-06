package main

import (
	"context"
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

	stopHeartbeat func() // nil until startup() runs
	priorCrash    logging.PriorCrashInfo
}

// NewApp creates a new App application struct
func NewApp(logRing *logging.RingHandler, logFile *os.File) *App {
	return &App{logRing: logRing, logFile: logFile}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	slog.Info("Porcupin starting up")

	// Setup data directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Error("Failed to get user home dir", "error", err)
		os.Exit(1)
	}
	dataDir := filepath.Join(homeDir, ".porcupin")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("Failed to create data dir", "error", err)
		os.Exit(1)
	}

	// Check for evidence the previous run died ungracefully (SIGKILL/OOM/power
	// loss). Reads only — does not modify the marker. We DEFER the actual
	// heartbeat start until critical startup succeeds (see below) so that a
	// failed-startup os.Exit doesn't leave a marker that would later be
	// misread as a runtime crash.
	a.priorCrash = logging.CheckPriorCrash(dataDir)
	if a.priorCrash.Detected {
		// Only format LastSeen if we actually parsed one from the marker —
		// otherwise the zero time logs as a misleading 0001-01-01.
		lastSeen := "unknown"
		if !a.priorCrash.LastSeen.IsZero() {
			lastSeen = a.priorCrash.LastSeen.Format(time.RFC3339)
		}
		slog.Warn("previous Porcupin run did not shut down cleanly",
			"prior_pid", a.priorCrash.PID,
			"last_seen", lastSeen,
			"hint", "likely OOM kill, system shutdown, or crash with no panic recovery")
	}

	// Load configuration
	configPath := filepath.Join(dataDir, "config.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Warn("Failed to load config, using defaults", "error", err)
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
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}

	if err := db.InitDB(gormDB); err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	a.database = db.NewDatabase(gormDB)
	slog.Info("Database initialized")

	// Initialize IPFS node
	// Ensure repo path is absolute
	repoPath := cfg.IPFS.RepoPath
	if strings.HasPrefix(repoPath, "~/") {
		repoPath = filepath.Join(homeDir, repoPath[2:])
	}

	ipfsNode, err := ipfs.NewNode(repoPath, cfg.IPFS.SwarmPort)
	if err != nil {
		slog.Error("Failed to create IPFS node", "error", err)
		os.Exit(1)
	}

	if err := ipfsNode.Start(ctx); err != nil {
		slog.Error("Failed to start IPFS node", "error", err)
		wailsRuntime.MessageDialog(ctx, wailsRuntime.MessageDialogOptions{
			Type:    wailsRuntime.ErrorDialog,
			Title:   "Startup Failed",
			Message: "Could not start the IPFS node.\n\n" + err.Error(),
		})
		os.Exit(1)
	}

	a.ipfsNode = ipfsNode
	slog.Info("IPFS node started")

	// Critical startup succeeded — start the heartbeat marker now. Earlier
	// failures (homeDir, mkdir, config, db, IPFS init) exit via os.Exit(1)
	// before we get here, so they leave no marker to poison the next launch.
	a.stopHeartbeat = logging.StartHeartbeat(ctx, dataDir)

	// Initialize indexer
	a.indexer = indexer.NewIndexer(cfg.TZKT.BaseURL)
	slog.Info("Indexer initialized")

	// Initialize backup service (handles automatic syncing)
	a.backupService = core.NewBackupService(ipfsNode, a.indexer, a.database, cfg)
	slog.Info("Backup service initialized")

	// Initialize updater
	updaterMgr, err := updater.NewManager(version.Version)
	if err != nil {
		slog.Warn("Failed to initialize updater", "error", err)
	} else {
		a.updater = updaterMgr
		slog.Info("Updater initialized", "version", version.Version)

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
	slog.Info("Backup service started, auto-syncing enabled")

	slog.Info("Porcupin startup complete")
}

// shutdown is called during application termination
func (a *App) shutdown(ctx context.Context) {
	// Hard deadline: guarantee the process exits even if IPFS close hangs or
	// WebKitGTK lingers. ShutdownTimeout is 30s for IPFS; 35s gives it a fair
	// chance before we force exit and let the OS reclaim all ports.
	go func() {
		time.Sleep(35 * time.Second)
		slog.Error("Forced process exit — shutdown exceeded 35 seconds")
		os.Exit(1)
	}()

	slog.Info("Porcupin shutting down")

	if a.backupService != nil {
		a.backupService.Stop()
	}

	if a.ipfsNode != nil {
		if err := a.ipfsNode.Stop(); err != nil {
			slog.Error("Error stopping IPFS node", "error", err)
		}
	}

	// Stop heartbeat last so the marker survives if anything above hangs —
	// then remove it cleanly to signal "this exit was intentional".
	if a.stopHeartbeat != nil {
		a.stopHeartbeat()
	}

	slog.Info("Porcupin shutdown complete")

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

	// Tell the frontend if the previous run died ungracefully. Frontend can
	// listen for "app:prior-crash" and surface a one-shot notice. Fields
	// (pid, last_seen) are omitted when unknown so the frontend renders
	// "unknown" rather than zero values.
	if a.priorCrash.Detected {
		payload := map[string]interface{}{}
		if a.priorCrash.PID > 0 {
			payload["pid"] = a.priorCrash.PID
		}
		if !a.priorCrash.LastSeen.IsZero() {
			payload["last_seen"] = a.priorCrash.LastSeen.Format(time.RFC3339)
		}
		wailsRuntime.EventsEmit(ctx, "app:prior-crash", payload)
	}
}

// beforeClose is called when the application is about to quit
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

// GetStatus returns the current status of the application
func (a *App) GetStatus() map[string]interface{} {
	stats, err := a.database.GetAssetStats()
	if err != nil {
		slog.Warn("GetStatus: failed to get asset stats", "error", err)
	}
	wallets, err := a.GetWallets()
	if err != nil {
		slog.Warn("GetStatus: failed to get wallets", "error", err)
	}

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
// Uses background context so health checks remain valid during shutdown.
func (a *App) GetIPFSHealth() ipfs.NodeHealthResult {
	if a.ipfsNode == nil {
		return ipfs.NodeHealthResult{IsOnline: false, Message: "Node not initialized"}
	}
	return a.ipfsNode.Health(context.Background())
}
