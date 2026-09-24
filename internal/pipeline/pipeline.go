// Package pipeline orchestrates a full plugin scan: wincmd.ini discovery,
// plugin parsing, version enrichment, user overrides and catalog checks.
// It is the single data-gathering path shared by the CLI and the GUI.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"totalplug/internal/checker"
	"totalplug/internal/config"
	"totalplug/internal/domain"
	"totalplug/internal/scanner"
	"totalplug/internal/storage"
)

// Options controls a single scan run.
type Options struct {
	IniPath      string // explicit wincmd.ini for portable installs; empty = auto-detect
	ForceRefresh bool   // bypass the 24h catalog cache
}

// Result carries everything an interface needs to render a scan.
type Result struct {
	IniPath    string
	TCDir      string
	Plugins    []domain.Plugin
	FromCache  bool
	CatalogErr error // non-fatal: catalog unreachable, plugins may be unresolved
}

// ResolveIni determines the wincmd.ini location and the Total Commander root
// directory used to expand plugin paths. An empty flagPath triggers auto
// discovery via Registry and AppData.
func ResolveIni(flagPath string) (iniPath string, tcDir string, err error) {
	if flagPath == "" {
		iniPath, tcDir = config.FindDefaultIniPath("")
		if iniPath == "" {
			return "", "", fmt.Errorf("could not automatically locate wincmd.ini via Registry or AppData; pass --ini <path> for portable installs")
		}
		return iniPath, tcDir, nil
	}

	abs, err := filepath.Abs(flagPath)
	if err != nil {
		return "", "", fmt.Errorf("resolving ini path: %w", err)
	}
	if info, statErr := os.Stat(abs); statErr != nil || info.IsDir() {
		return "", "", fmt.Errorf("wincmd.ini not accessible at %s", abs)
	}

	// A wincmd.exe next to the ini marks a portable install; otherwise fall back
	// to the registry install dir for non-portable layouts.
	tcDir = filepath.Dir(abs)
	if _, statErr := os.Stat(filepath.Join(tcDir, "wincmd.exe")); statErr != nil {
		if _, regDir := config.FindIniFromRegistry(); regDir != "" {
			tcDir = regDir
		}
	}
	return abs, tcDir, nil
}

// Scan discovers installed plugins, enriches them with versions and user
// overrides, and resolves updates against the community catalog. A catalog
// failure is non-fatal and reported via Result.CatalogErr. A nil db simply
// skips user overrides.
func Scan(ctx context.Context, opts Options, db *storage.Database) (Result, error) {
	iniPath, tcDir, err := ResolveIni(opts.IniPath)
	if err != nil {
		return Result{}, err
	}

	plugins, err := config.ParsePluginsFromIni(iniPath, tcDir)
	if err != nil {
		return Result{}, fmt.Errorf("parsing wincmd.ini: %w", err)
	}

	plugins = scanner.EnrichPluginVersions(plugins)

	if db != nil {
		plugins = db.ApplyOverrides(plugins)
	}

	chk := checker.NewChecker(opts.ForceRefresh)
	resolved, fromCache, catalogErr := chk.CheckUpdates(ctx, plugins)
	if catalogErr == nil {
		plugins = resolved
	}

	return Result{
		IniPath:    iniPath,
		TCDir:      tcDir,
		Plugins:    plugins,
		FromCache:  fromCache,
		CatalogErr: catalogErr,
	}, nil
}
