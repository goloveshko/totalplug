// File: app.go
package main

import (
	"context"
	"fmt"

	"totalplug/internal/checker"
	"totalplug/internal/config"
	"totalplug/internal/domain"
	"totalplug/internal/scanner"
	"totalplug/internal/storage"
	"totalplug/internal/updater"
)

// App struct manages application state and UI bindings
type App struct {
	ctx     context.Context
	db      *storage.Database
	checker *checker.Checker
	updater *updater.Updater
}

// NewApp creates a new App application struct
func NewApp() *App {
	db, err := storage.NewDatabase("")
	if err != nil {
		fmt.Printf("[!] Warning initializing database: %v\n", err)
	}

	return &App{
		db:      db,
		checker: checker.NewChecker(false),
		updater: updater.NewUpdater(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ScanPlugins scans wincmd.ini, extracts versions, and queries the catalog
func (a *App) ScanPlugins(forceRefresh bool) ([]domain.Plugin, error) {
	iniPath, tcDir := config.FindDefaultIniPath("")
	if iniPath == "" {
		return nil, fmt.Errorf("could not locate wincmd.ini via Registry or AppData")
	}

	plugins, err := config.ParsePluginsFromIni(iniPath, tcDir)
	if err != nil {
		return nil, fmt.Errorf("parsing wincmd.ini: %w", err)
	}

	plugins = scanner.EnrichPluginVersions(plugins)

	if a.db != nil {
		plugins = a.db.ApplyOverrides(plugins)
	}

	chk := checker.NewChecker(forceRefresh)
	resolved, _, err := chk.CheckUpdates(a.ctx, plugins)
	if err != nil {
		return plugins, fmt.Errorf("catalog check warning: %w", err)
	}

	return resolved, nil
}

// SaveOverride sets manual name/version/source override for a plugin
func (a *App) SaveOverride(pluginID string, override domain.Override) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.db.SetUserOverride(pluginID, override)
}

// UpdatePlugin installs the latest version for the specified plugin
func (a *App) UpdatePlugin(plugin domain.Plugin) error {
	_, err := a.updater.UpdatePlugin(a.ctx, plugin)
	return err
}