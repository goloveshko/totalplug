package main

import (
	"embed"

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

	// Frameless window with an optional translucent backdrop (Mica/Acrylic).
	// With an effect active the webview must be transparent for it to show.
	effect := loadWindowEffect()
	var backdrop windows.BackdropType
	switch effect {
	case effectMica:
		backdrop = windows.Mica
	case effectAcrylic:
		backdrop = windows.Acrylic
	}
	translucent := backdrop != windows.None

	background := &options.RGBA{R: 244, G: 244, B: 247, A: 255}
	if translucent {
		background = &options.RGBA{A: 0}
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "totalplug",
		Width:     1024,
		Height:    768,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: background,
		OnStartup:        app.startup,
		Windows: &windows.Options{
			WebviewIsTransparent: translucent,
			WindowIsTranslucent:  translucent,
			BackdropType:         backdrop,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
