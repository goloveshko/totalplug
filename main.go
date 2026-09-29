package main

import (
	"embed"
	"fmt"

	"totalplug/internal/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Frameless window with a frontend-drawn titlebar. The Mica/Acrylic backdrop
	// is temporarily disabled: a translucent window washes out the themed UI,
	// and a proper effects pass needs its own design work (see ROADMAP).
	// Create application with options
	err := wails.Run(&options.App{
		Title:     "totalplug",
		Width:     1024,
		Height:    768,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 244, G: 244, B: 247, A: 255},
		OnStartup:        app.startup,
		Windows: &windows.Options{
			WebviewUserDataPath: webviewUserDataPath(),
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// webviewUserDataPath keeps the WebView2 profile — and with it the frontend's
// localStorage: theme, language, font scale — inside the data directory, so a
// portable folder carries those choices too. An empty answer leaves Wails with
// its own default (%APPDATA%\totalplug.exe), which is what a run that cannot
// write anywhere would fall back to anyway.
func webviewUserDataPath() string {
	dir, err := config.WebviewDataDir()
	if err != nil {
		fmt.Printf("[!] WebView2 data stays in the Wails default location: %v\n", err)
		return ""
	}
	return dir
}
