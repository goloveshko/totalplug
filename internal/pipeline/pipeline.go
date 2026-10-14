// Package pipeline orchestrates a full plugin scan: wincmd.ini discovery,
// plugin parsing, version enrichment, user overrides and catalog checks.
// It is the single data-gathering path shared by the CLI and the GUI.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goloveshko/totalplug/internal/checker"
	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/scanner"
	"github.com/goloveshko/totalplug/internal/storage"
)

// Options controls a single scan run.
type Options struct {
	IniPath      string // explicit wincmd.ini for portable installs; empty = auto-detect
	ForceRefresh bool   // bypass the 24h catalog cache
	CacheOnly    bool   // never touch the network; catalog comes from disk cache
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
			return "", "", fmt.Errorf("could not automatically locate wincmd.ini via Registry or AppData; point TotalPlug at it (Settings > Total Commander, or --ini <path> on the command line)")
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

// buildLegacyIDMap maps old-style "<TYPE>_<name>" IDs to the current stable
// IDs for freshly parsed plugins. Legacy IDs claimed by several plugins are
// dropped so migration never guesses which plugin user data belongs to.
func buildLegacyIDMap(plugins []domain.Plugin) map[string]string {
	counts := make(map[string]int)
	mapping := make(map[string]string)
	for _, p := range plugins {
		legacy := config.LegacyPluginID(p.Type, p.Name)
		counts[legacy]++
		mapping[legacy] = p.ID
	}
	for legacy, n := range counts {
		if n > 1 {
			delete(mapping, legacy)
		}
	}
	return mapping
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
		if _, err := db.MigrateLegacyIDs(buildLegacyIDMap(plugins)); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Could not migrate legacy plugin data: %v\n", err)
		}
		plugins = db.ApplyOverrides(plugins)
	}

	chk := checker.NewChecker(opts.ForceRefresh, opts.CacheOnly)
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
