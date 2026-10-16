package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/goloveshko/totalplug/internal/config"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The window is frameless and drawn by the frontend, but its geometry has to be
// known before wails.Run creates the HWND, so it lives in a settings file next
// to the other data instead of the frontend's localStorage.
//
// The file is shared with the CLI, which reads the backup retention policy from
// it before any window exists — hence the policy type comes from internal/config
// instead of being copied into this package.

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

// settings is the persisted state. Window geometry is what the next launch
// restores; WindowEffect is the backdrop a future effects pass will apply, kept
// here because it, like the size, has to be known before the window exists. The
// theme, language and font scale stay in the frontend's localStorage.
type settings struct {
	Window       windowGeometry `json:"window"`
	WindowEffect string         `json:"window_effect"`
	// TcIni is a manual wincmd.ini, for a Total Commander the registry does not
	// know about. Empty means auto-detect. It belongs to this file rather than
	// localStorage because the scanner reads it before any page exists, and a
	// portable install then carries the choice along with the data.
	TcIni string `json:"tc_ini,omitempty"`
	// Backup is the retention policy the updater applies. It rides in this file
	// because the CLI has to honour the same answer, and the CLI has no window to
	// read a GUI-only settings struct through. It is a pointer so that a file
	// written before these keys existed stays distinguishable from one where the
	// user deliberately chose 0 days — the first means defaults, the second means
	// no age limit.
	Backup *config.BackupPolicy `json:"backup,omitempty"`
}

// backupPolicy answers with the stored policy, or the defaults when the file has
// no opinion yet.
func (s settings) backupPolicy() config.BackupPolicy {
	if s.Backup == nil {
		return config.DefaultBackupPolicy()
	}
	return *s.Backup
}

func settingsPath() string {
	return config.SettingsPath()
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
func writeSettings(s settings) error {
	path := settingsPath()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write settings to %s: %w", path, err)
	}
	return nil
}

// seedSettings primes the mirrored state from the saved file before the window
// exists, so a close before the first sample keeps the stored geometry and the
// stored backdrop.
func (a *App) seedSettings(s settings) {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	a.state = s
}

// snapshotSettings returns the most recently mirrored state.
func (a *App) snapshotSettings() settings {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.state
}

// setWindowEffect replaces the backdrop choice and persists it right away, so
// the value survives even if the app dies before a close writes the mirror.
func (a *App) setWindowEffect(effect string) error {
	a.settingsMu.Lock()
	a.state.WindowEffect = effect
	s := a.state
	a.settingsMu.Unlock()
	return writeSettings(s)
}

// setTcIni stores the manual wincmd.ini choice, persisted right away for the
// same reason the backdrop is: the next scan reads it, and a crash should not
// silently return the user to auto-detection.
func (a *App) setTcIni(path string) error {
	a.settingsMu.Lock()
	a.state.TcIni = path
	s := a.state
	a.settingsMu.Unlock()
	return writeSettings(s)
}

// setBackupPolicy stores the retention choice and persists it right away,
// because the next updater — which may be a CLI run, not this window — reads it
// from the file rather than from anything the frontend knows.
func (a *App) setBackupPolicy(policy config.BackupPolicy) error {
	policy = config.NormalizeBackupPolicy(policy)
	a.settingsMu.Lock()
	a.state.Backup = &policy
	s := a.state
	a.settingsMu.Unlock()
	return writeSettings(s)
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
		a.settingsMu.Lock()
		a.state.Window.Maximised = true
		a.settingsMu.Unlock()
		return
	}

	width, height := wailsruntime.WindowGetSize(a.ctx)
	if width <= 0 || height <= 0 {
		return
	}
	x, y := wailsruntime.WindowGetPosition(a.ctx)

	a.settingsMu.Lock()
	a.state.Window.Width, a.state.Window.Height = width, height
	a.state.Window.X, a.state.Window.Y = x, y
	a.state.Window.Maximised = false
	a.settingsMu.Unlock()
}
