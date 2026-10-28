// File: app.go
package gui

import (
	"context"
	"errors"
	"fmt"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/goloveshko/totalplug/internal/buildinfo"
	"github.com/goloveshko/totalplug/internal/checker"
	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/lock"
	"github.com/goloveshko/totalplug/internal/pipeline"
	"github.com/goloveshko/totalplug/internal/rollback"
	"github.com/goloveshko/totalplug/internal/storage"
	"github.com/goloveshko/totalplug/internal/updater"

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

// GetUpdateHistory returns the changes the updater applied, newest first.
//
// Only the newest line about a plugin still describes the folder as it stands, so
// only that one keeps the copy it names; rollback.ClearStaleCopies drops the rest,
// and is the same rule the command line applies when it lists the log.
func (a *App) GetUpdateHistory() ([]domain.UpdateEntry, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	entries := a.db.UpdateHistory()
	rollback.ClearStaleCopies(entries)
	return entries, nil
}

// RollbackUpdate puts the copy one history line kept back into its plugin folder
// and logs the change. It takes the lock an update takes, so a rollback cannot
// cross an install in progress, and a folder Total Commander still holds reports
// through the structured result the same way an update does.
func (a *App) RollbackUpdate(entry domain.UpdateEntry) (domain.UpdateResult, error) {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()

	if a.db == nil {
		return domain.UpdateResult{}, fmt.Errorf("database not initialized")
	}
	restored, err := rollback.Apply(a.updater, a.db, entry)
	if err != nil {
		if errors.Is(err, updater.ErrFileLocked) {
			return domain.UpdateResult{Locked: true}, nil
		}
		return domain.UpdateResult{}, err
	}
	snapshot := restored.SnapshotPath

	if err := a.db.RecordChange(storage.ChangeFromRestore(entry, snapshot)); err != nil {
		fmt.Printf("[!] Could not log the rollback of %s: %v\n", entry.PluginID, err)
	}
	// The folder holds the older version again either way. A file the newer one
	// brought that another process still holds stays where it is until that process
	// lets go, which is no reason to report a rollback that finished as a failure.
	for _, name := range restored.LeftBehind {
		fmt.Printf("[!] Could not take %s out of %s: it is still held open\n", name, entry.PluginDir)
	}
	return domain.UpdateResult{BackupPath: snapshot}, nil
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

// GetWindowEffect reports the backdrop the window was actually created with
// ("mica" or "none"): DWM applies it before the page exists, and a build below
// Windows 11 22H2 gets the solid background. The frontend uses this to decide
// whether its own surfaces may stay transparent.
func (a *App) GetWindowEffect() string {
	return backdrop
}

// GetBackupPolicy returns the retention the updater applies to the copies it
// leaves behind.
func (a *App) GetBackupPolicy() config.BackupPolicy {
	return a.snapshotSettings().backupPolicy()
}

// SetBackupPolicy stores the retention choice. It reaches the settings file at
// once because the updater that reads it may be a CLI run rather than this
// window.
func (a *App) SetBackupPolicy(policy config.BackupPolicy) error {
	return a.setBackupPolicy(policy)
}

// BackupLocation is the folder the copies live in. It is shown so the user knows
// where to look before deleting anything, and it is not created by being named.
func (a *App) BackupLocation() string {
	return config.BackupSpacePath()
}

// UpdatePlugin installs the latest version for the specified plugin. A locked
// target (Total Commander still running) and a refused plain-HTTP source are
// reported through the structured result instead of an error, so the frontend
// can show recovery guidance and ask the one question it needs answered.
func (a *App) UpdatePlugin(plugin domain.Plugin) (domain.UpdateResult, error) {
	return a.updatePlugin(plugin, false)
}

// UpdatePluginInsecure is UpdatePlugin for a plugin whose download the user
// agreed to take over plain HTTP. The frontend asks first.
func (a *App) UpdatePluginInsecure(plugin domain.Plugin) (domain.UpdateResult, error) {
	return a.updatePlugin(plugin, true)
}

func (a *App) updatePlugin(plugin domain.Plugin, allowInsecure bool) (domain.UpdateResult, error) {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()

	// One writer at a time per data folder. The window and the command line
	// can both be open, and two runs replacing the same plugin folder leave a
	// plugin that is half one version and half the other.
	guard, err := lock.Acquire(config.DataDir(), 10*time.Second)
	if err != nil {
		return domain.UpdateResult{}, err
	}
	defer guard.Release()

	a.updater.SetProgressListener(func(p domain.UpdateProgress) {
		runtime.EventsEmit(a.ctx, updateProgressEvent, p)
	})
	defer a.updater.SetProgressListener(nil)

	var outcome updater.Outcome
	if allowInsecure {
		outcome, err = a.updater.UpdatePluginInsecure(a.ctx, plugin)
	} else {
		outcome, err = a.updater.UpdatePlugin(a.ctx, plugin)
	}
	if err != nil {
		if errors.Is(err, updater.ErrFileLocked) {
			return domain.UpdateResult{Locked: true}, nil
		}
		if errors.Is(err, updater.ErrPlainHTTPSource) {
			return domain.UpdateResult{Insecure: true}, nil
		}
		return domain.UpdateResult{}, err
	}

	// Remember the applied version so blind (unknown-installed) plugins do not
	// resurface as uncertain on the next scan.
	if a.db != nil {
		if err := a.db.RecordChange(storage.ChangeFromUpdate(plugin, outcome.BackupPath, outcome.AppliedFiles)); err != nil {
			fmt.Printf("[!] Could not remember version %s for %s: %v\n", plugin.LatestVersion, plugin.ID, err)
		}
	}

	return domain.UpdateResult{BackupPath: outcome.BackupPath}, nil
}
