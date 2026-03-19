package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"porcupin/backend/logging"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	// --- Logging setup ---
	// Determine data directory early so we can open the log file before anything else.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Cannot determine home directory: %v", err)
	}
	dataDir := filepath.Join(homeDir, ".porcupin")
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0755); err != nil {
		log.Printf("Warning: could not create logs directory: %v", err)
	}

	ringHandler := logging.NewRingHandler(1000, slog.LevelInfo)

	logFile, err := logging.OpenLogFile(dataDir, 7)
	if err != nil {
		log.Printf("Warning: could not open log file: %v", err)
	}

	// Build fan-out handler: stderr + ring buffer + optional file
	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stderr, nil),
		ringHandler,
	}
	if logFile != nil {
		handlers = append(handlers, slog.NewTextHandler(logFile, nil))
	}
	slog.SetDefault(slog.New(logging.NewMultiHandler(handlers...)))
	// Bridge the standard library log package so every log.Printf/log.Println call
	// flows through slog into the ring buffer, log file, and stderr handler.
	log.SetOutput(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
	log.SetFlags(0) // slog adds its own timestamps; suppress log package's prefix

	// Crash recovery: write a report file if main() panics, then exit non-zero
	// so the OS/systemd registers the process as crashed.
	defer func() {
		if r := recover(); r != nil {
			logging.WriteCrashReport(dataDir, r, ringHandler)
			os.Exit(1)
		}
	}()

	// Check for debug mode via environment variable
	debugMode := os.Getenv("PORCUPIN_DEBUG") == "1"

	// Create an instance of the app structure
	app := NewApp(ringHandler, logFile)

	// Create application menu
	appMenu := menu.NewMenu()

	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("Show Dashboard", keys.CmdOrCtrl("d"), func(_ *menu.CallbackData) {
		runtime.WindowShow(app.ctx)
		runtime.WindowUnminimise(app.ctx)
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		runtime.Quit(app.ctx)
	})

	appMenu.Append(menu.EditMenu())

	// Create application with options
	runErr := wails.Run(&options.App{
		Title:             "Porcupin - Tezos NFT Backup",
		Width:             1024,
		Height:            768,
		MinWidth:          800,
		MinHeight:         600,
		StartHidden:       false,
		HideWindowOnClose: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 255},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Menu:             appMenu,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Porcupin",
				Message: "Version 1.0.0\n\nTezos NFT Backup to IPFS\n\nDeveloped by skllzrmy.tez\n\nGitHub: github.com/skllzrmy/porcupin\n\nSupport: joe@poundfit.com",
				Icon:    icon,
			},
		},
		// Enable DevTools in debug mode (set PORCUPIN_DEBUG=1 env var)
		Debug: options.Debug{
			OpenInspectorOnStartup: debugMode,
		},
	})

	if runErr != nil {
		// OnShutdown was never called — close the log file to avoid fd leak.
		if logFile != nil {
			logFile.Close()
		}
		println("Error:", runErr.Error())
	}
}
