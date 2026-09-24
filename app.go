// File: app.go
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
)

// App struct manages application state and UI bindings
type App struct {
	ctx     context.Context
	db      *storage.Database
	updater *updater.Updater

	refreshMu  sync.Mutex
	refreshing bool
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
}

// ScanPlugins returns an immediate cache-first scan: the catalog comes from
// disk without any network call, so the table renders even on first paint or
// offline. A background full check follows and pushes fresh data through the
// plugins:refreshed event.
func (a *App) ScanPlugins(forceRefresh bool) ([]domain.Plugin, error) {
	res, err := pipeline.Scan(a.ctx, pipeline.Options{ForceRefresh: forceRefresh, CacheOnly: true}, a.db)
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

		res, err := pipeline.Scan(ctx, pipeline.Options{ForceRefresh: force}, a.db)
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

// UpdatePlugin installs the latest version for the specified plugin. A locked
// target (Total Commander still running) is reported through the structured
// result instead of an error so the frontend can show recovery guidance.
func (a *App) UpdatePlugin(plugin domain.Plugin) (domain.UpdateResult, error) {
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
		if err := a.db.RememberVersion(plugin.ID, plugin.LatestVersion); err != nil {
			fmt.Printf("[!] Could not remember version %s for %s: %v\n", plugin.LatestVersion, plugin.ID, err)
		}
	}

	return domain.UpdateResult{BackupPath: backupDir}, nil
}
