package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"totalplug/internal/config"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The window is frameless and drawn by the frontend, but its geometry has to be
// known before wails.Run creates the HWND, so it lives in a settings file next
// to the other data instead of the frontend's localStorage.
const settingsFileName = "settings.json"

// Default geometry only applies before a settings file exists.
const (
	defaultWidth  = 1024
	defaultHeight = 768

	// windowSampleInterval is how often the live geometry is mirrored. The
	// mirror exists because a closing window answers size/position queries with
	// zeros, so the value written at shutdown has to come from recent history.
	windowSampleInterval = time.Second
)

// windowGeometry is what the next launch restores. While the window is maximised
// only Maximised is meaningful: the live size is the whole display, so the last
// floating geometry is carried over untouched and comes back as a sane window.
type windowGeometry struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

// settings is the persisted state. Window geometry is the first member; the
// theme, language and font scale stay in the frontend's localStorage until a
// settings view writes there.
type settings struct {
	Window windowGeometry `json:"window"`
}

func settingsPath() string {
	return filepath.Join(config.DataDir(), settingsFileName)
}

// loadSettings returns the saved settings and whether a readable file existed.
// A missing or corrupt file is not fatal: the caller falls back to the defaults.
func loadSettings() (settings, bool) {
	path := settingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return settings{}, false
	}
	var s settings
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Printf("[!] Ignoring corrupt settings in %s: %v\n", path, err)
		return settings{}, false
	}
	return s, true
}

// writeSettings persists the settings. It queries no window, which is what makes
// it safe to call while the app is tearing down.
func writeSettings(s settings) {
	path := settingsPath()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Printf("[!] Could not encode settings: %v\n", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("[!] Could not save settings in %s: %v\n", path, err)
	}
}

// seedWindowState primes the tracked geometry from the saved state before the
// window exists, so a close before the first sample keeps the stored size.
func (a *App) seedWindowState(geometry windowGeometry) {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	a.winState = geometry
}

// windowSnapshot returns the most recently tracked geometry.
func (a *App) windowSnapshot() windowGeometry {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()
	return a.winState
}

// trackWindowLoop mirrors the window geometry while the app runs. Bindings and
// background scans already work off other goroutines, so this only reads.
func (a *App) trackWindowLoop() {
	ticker := time.NewTicker(windowSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.trackWindow()
		}
	}
}

func (a *App) trackWindow() {
	if wailsruntime.WindowIsMaximised(a.ctx) {
		a.windowMu.Lock()
		a.winState.Maximised = true
		a.windowMu.Unlock()
		return
	}

	width, height := wailsruntime.WindowGetSize(a.ctx)
	if width <= 0 || height <= 0 {
		return
	}
	x, y := wailsruntime.WindowGetPosition(a.ctx)

	a.windowMu.Lock()
	a.winState.Width, a.winState.Height = width, height
	a.winState.X, a.winState.Y = x, y
	a.winState.Maximised = false
	a.windowMu.Unlock()
}
