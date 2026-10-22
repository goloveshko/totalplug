// File: run.go
package gui

import (
	"context"
	"embed"
	"fmt"

	"github.com/goloveshko/totalplug/internal/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// minWidth and minHeight are the smallest window that still shows the app: below
// 880 the titlebar's count chip wraps onto two lines and the plugin table starts
// scrolling sideways, measured at the largest font scale the Settings panel offers
// (140%), which is what the rem-sized titlebar grows with. Wails scales both
// against the window's DPI, so these are logical pixels.
const (
	minWidth  = 880
	minHeight = 480
)

// Run opens the window over the embedded frontend and blocks until it closes.
// The assets come in as an argument because //go:embed only reaches below the
// directory of the file that declares it, and frontend/ sits at the repo root —
// which is also where wails build compiles from.
func Run(assets embed.FS) error {
	app := NewApp()

	// Restore the last size, position and maximised state; a first run gets the
	// compiled-in defaults. The whole loaded state is mirrored in the app, so the
	// save on close cannot drop the Total Commander path.
	loaded, hasSettings := loadSettings()
	saved := loaded.Window
	width, height := defaultWidth, defaultHeight
	if hasSettings && saved.Width > 0 && saved.Height > 0 {
		width, height = saved.Width, saved.Height
	}
	app.seedSettings(loaded)

	// Frameless window with a frontend-drawn titlebar. On Windows 11 22H2+ the
	// window gets the Mica backdrop and the page stays transparent above it; on
	// anything older the same layout sits on a solid background. The attribute
	// cannot be changed after creation, which is why there is no toggle.
	background := &options.RGBA{R: 244, G: 244, B: 247, A: 255}
	windowsOptions := &windows.Options{
		WebviewUserDataPath: webviewUserDataPath(),
	}
	if backdrop == backdropMica {
		background = &options.RGBA{}
		windowsOptions.BackdropType = windows.Mica
		windowsOptions.WebviewIsTransparent = true
		windowsOptions.WindowIsTranslucent = true
	}

	return wails.Run(&options.App{
		Title:  "totalplug",
		Width:  width,
		Height: height,
		// A geometry saved before the limit existed is clamped here by Wails' own
		// validation, so the first launch after this change opens usable whatever
		// settings.json says.
		MinWidth:  minWidth,
		MinHeight: minHeight,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: background,
		// Width and Height place the window where the OS puts a new one; the
		// saved position can only be applied once the HWND exists.
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			if hasSettings && !saved.Maximised && (saved.X != 0 || saved.Y != 0) {
				wailsruntime.WindowSetPosition(ctx, saved.X, saved.Y)
			}
		},
		// A frameless window does not come up maximised from WindowStartState;
		// the maximise has to be requested once the HWND exists.
		OnDomReady: func(ctx context.Context) {
			if saved.Maximised {
				wailsruntime.WindowMaximise(ctx)
			}
		},
		// Persist from the mirror, not from the window: a closing window answers
		// size and position queries with zeros.
		OnBeforeClose: func(context.Context) bool {
			if err := writeSettings(app.snapshotSettings()); err != nil {
				fmt.Printf("[!] %v\n", err)
			}
			return false
		},
		// The download scratch is the app's only %TEMP% writing, and it is the one
		// thing a closed window should not leave behind.
		OnShutdown: func(context.Context) {
			config.ReleaseRunScratch()
		},
		Windows: windowsOptions,
		Bind: []interface{}{
			app,
		},
	})
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
