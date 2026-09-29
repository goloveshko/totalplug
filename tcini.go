// File: tcini.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"totalplug/internal/domain"
	"totalplug/internal/pipeline"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// iniFilters narrows the picker to a Total Commander config without hiding the
// case it exists for: a portable TC keeps wincmd.ini next to wincmd.exe, and an
// ini taken from elsewhere can carry any name.
var iniFilters = []runtime.FileFilter{
	{DisplayName: "Total Commander configuration (wincmd.ini)", Pattern: "wincmd.ini"},
	{DisplayName: "All files", Pattern: "*.*"},
}

// GetTcConfig tells the Settings panel which wincmd.ini the next scan opens.
func (a *App) GetTcConfig() domain.TcConfig {
	manual := a.snapshotSettings().TcIni
	cfg := domain.TcConfig{Configured: manual, Effective: manual}

	// Resolution is cheap (registry reads and two stat calls) and the panel has
	// to show it even when a manual path wins, so the user can jump back.
	detected, _, err := pipeline.ResolveIni("")
	if err == nil {
		cfg.Detected = detected
	}
	if cfg.Effective == "" {
		cfg.Effective = cfg.Detected
	}
	if cfg.Effective != "" {
		if info, statErr := os.Stat(cfg.Effective); statErr == nil && !info.IsDir() {
			cfg.Readable = true
		}
	}
	return cfg
}

// SetTcConfig stores a manual wincmd.ini path; empty returns to auto-detection.
// The path is resolved and checked here so a typo is reported while the field
// still has the user's attention, instead of surfacing as a failed scan later.
func (a *App) SetTcConfig(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return a.setTcIni("")
	}
	resolved, _, err := pipeline.ResolveIni(trimmed)
	if err != nil {
		return err
	}
	return a.setTcIni(resolved)
}

// BrowseTcIni opens the native picker over the folder the current config lives
// in. The title comes from the caller because it is the one string in this
// binding the user reads in their own language. An empty result means the user
// cancelled, which is not an error.
func (a *App) BrowseTcIni(title string) (string, error) {
	options := runtime.OpenDialogOptions{
		Title:                title,
		Filters:              iniFilters,
		CanCreateDirectories: true,
		ResolvesAliases:      true,
	}
	// Wails turns a missing DefaultDirectory into an error, so only offer one
	// that is actually a folder.
	if dir := filepath.Dir(a.GetTcConfig().Effective); dir != "" && dir != "." {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			options.DefaultDirectory = dir
		}
	}

	path, err := runtime.OpenFileDialog(a.ctx, options)
	if err != nil {
		return "", fmt.Errorf("open the file dialog: %w", err)
	}
	return path, nil
}
