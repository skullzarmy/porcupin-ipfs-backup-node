package main

import (
	"embed"
	_ "embed"
	"os"

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
	// Check for debug mode via environment variable
	debugMode := os.Getenv("PORCUPIN_DEBUG") == "1"

	// Create an instance of the app structure
	app := NewApp()

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
	err := wails.Run(&options.App{
		Title:            "Porcupin - Tezos NFT Backup",
		Width:            1024,
		Height:           768,
		MinWidth:         800,
		MinHeight:        600,
		StartHidden:      false,
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
			TitleBar: mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
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

	if err != nil {
		println("Error:", err.Error())
	}
}
