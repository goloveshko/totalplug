// File: app.go
package gui

import (
	"context"
	"errors"
	"fmt"
	goruntime "runtime"
	"sync"
	"time"

	"totalplug/internal/buildinfo"
	"totalplug/internal/checker"
	"totalplug/internal/config"
	"totalplug/internal/domain"
	"totalplug/internal/pipeline"
	"totalplug/internal/storage"
	"totalplug/internal/updater"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// pluginsRefreshedEvent delivers background scan results to the frontend.
	pluginsRefreshed     = "plugins:refreshed"
	pluginsRefreshFailed = "plugins:refresh:failed"
	// updateProgressEvent streams updater phase/percent events to the progress dialog.
	updateProgressEvent = "update:progress"
)

// App struct manages application state and UI bindings
type App struct {
	ctx     context.Context
	db      *storage.Database
	updater *updater.Updater

	refreshMu  sync.Mutex
	refreshing bool

	// updateMu keeps one UpdatePlugin running at a time. The frontend already
	// awaits them in order, but a Wails binding is callable from anywhere: two
	// runs would fight over the single progress listener and over the one
	// scratch folder of this process, where the second run empties the package
	// the first is still unpacking.
	updateMu sync.Mutex

	// state mirrors the persisted settings: the window geometry is sampled while
	// the app runs so a teardown can save it without querying the window that is
	// already closing, and the backdrop choice changes while it runs too.
	settingsMu sync.Mutex
	state      settings
}

// NewApp creates a new App application struct
func NewApp() *App {
	db, err := storage.NewDatabase("")
	if err != nil {
		fmt.Printf("[!] Warning initializing database: %v\n", err)
	}

	return &App{
		db:      db,
		updater: updater.NewUpdater(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.trackWindowLoop()
}

// ScanPlugins returns an immediate cache-first scan: the catalog comes from
// disk without any network call, so the table renders even on first paint or
// offline. A background full check follows and pushes fresh data through the
// plugins:refreshed event.
func (a *App) ScanPlugins(forceRefresh bool) ([]domain.Plugin, error) {
	res, err := pipeline.Scan(a.ctx, pipeline.Options{
		ForceRefresh: forceRefresh,
		CacheOnly:    true,
		IniPath:      a.snapshotSettings().TcIni,
	}, a.db)
	if err != nil {
		return nil, err
	}
	a.startBackgroundRefresh(forceRefresh)
	return res.Plugins, nil
}

// startBackgroundRefresh runs one full network scan at a time and emits its
// result (or failure) as a Wails event.
func (a *App) startBackgroundRefresh(force bool) {
	a.refreshMu.Lock()
	if a.refreshing {
		a.refreshMu.Unlock()
		return
	}
	a.refreshing = true
	a.refreshMu.Unlock()

	go func() {
		defer func() {
			a.refreshMu.Lock()
			a.refreshing = false
			a.refreshMu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		res, err := pipeline.Scan(ctx, pipeline.Options{ForceRefresh: force, IniPath: a.snapshotSettings().TcIni}, a.db)
		switch {
		case err != nil:
			runtime.EventsEmit(a.ctx, pluginsRefreshFailed, err.Error())
		case res.CatalogErr != nil:
			runtime.EventsEmit(a.ctx, pluginsRefreshed, res.Plugins)
			runtime.EventsEmit(a.ctx, pluginsRefreshFailed, fmt.Sprintf("catalog unreachable: %v", res.CatalogErr))
		default:
			runtime.EventsEmit(a.ctx, pluginsRefreshed, res.Plugins)
		}
	}()
}

// SaveOverride sets manual name/version/source override for a plugin
func (a *App) SaveOverride(pluginID string, override domain.Override) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.db.SetUserOverride(pluginID, override)
}

// GetUpdateHistory returns successfully applied updates, newest first.
func (a *App) GetUpdateHistory() ([]domain.UpdateEntry, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return a.db.UpdateHistory(), nil
}

// AppInfo returns what the About panel shows without touching the network:
// the stamped build version and where published releases live.
func (a *App) AppInfo() domain.AppInfo {
	return domain.AppInfo{
		Version:     buildinfo.Version,
		ReleasesURL: buildinfo.ReleasesURL,
		DataDir:     config.DataDir(),
		GoVersion:   goruntime.Version(),
	}
}

// CheckForAppUpdate compares the running build with the latest published
// release. Nothing calls it automatically — it runs when the user presses
// "check for updates" in the About panel.
func (a *App) CheckForAppUpdate() (domain.AppUpdate, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return checker.CheckAppRelease(ctx, buildinfo.Version)
}

// GetWindowEffect returns the backdrop applied at startup ("none", "mica"
// or "acrylic"); the frontend mirrors it in the settings select and decides
// whether to render a transparent body.
func (a *App) GetWindowEffect() string {
	return normalizeWindowEffect(a.snapshotSettings().WindowEffect)
}

// SetWindowEffect persists the backdrop choice; it is read once before the
// window is created, so a restart is required to apply it.
func (a *App) SetWindowEffect(effect string) error {
	if !validWindowEffect(effect) {
		return fmt.Errorf("unknown window effect %q", effect)
	}
	return a.setWindowEffect(effect)
}

// UpdatePlugin installs the latest version for the specified plugin. A locked
// target (Total Commander still running) is reported through the structured
// result instead of an error so the frontend can show recovery guidance.
func (a *App) UpdatePlugin(plugin domain.Plugin) (domain.UpdateResult, error) {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()

	a.updater.SetProgressListener(func(p domain.UpdateProgress) {
		runtime.EventsEmit(a.ctx, updateProgressEvent, p)
	})
	defer a.updater.SetProgressListener(nil)

	backupDir, err := a.updater.UpdatePlugin(a.ctx, plugin)
	if err != nil {
		if errors.Is(err, updater.ErrFileLocked) {
			return domain.UpdateResult{Locked: true}, nil
		}
		return domain.UpdateResult{}, err
	}

	// Remember the applied version so blind (unknown-installed) plugins do not
	// resurface as uncertain on the next scan.
	if a.db != nil {
		if err := a.db.RememberVersion(plugin.ID, plugin.Name, plugin.LatestVersion); err != nil {
			fmt.Printf("[!] Could not remember version %s for %s: %v\n", plugin.LatestVersion, plugin.ID, err)
		}
	}

	return domain.UpdateResult{BackupPath: backupDir}, nil
}
