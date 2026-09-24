// File: app.go
package main

import (
	"context"
	"fmt"

	"totalplug/internal/domain"
	"totalplug/internal/pipeline"
	"totalplug/internal/storage"
	"totalplug/internal/updater"
)

// App struct manages application state and UI bindings
type App struct {
	ctx     context.Context
	db      *storage.Database
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
		updater: updater.NewUpdater(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ScanPlugins scans wincmd.ini, extracts versions, and queries the catalog
func (a *App) ScanPlugins(forceRefresh bool) ([]domain.Plugin, error) {
	res, err := pipeline.Scan(a.ctx, pipeline.Options{ForceRefresh: forceRefresh}, a.db)
	if err != nil {
		return nil, err
	}
	if res.CatalogErr != nil {
		return res.Plugins, fmt.Errorf("catalog check warning: %w", res.CatalogErr)
	}
	return res.Plugins, nil
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