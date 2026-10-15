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
// discovery via Registry and AppData. The root can come back empty, which means
// nothing vouched for a folder and %COMMANDER_PATH% cannot be expanded.
func ResolveIni(flagPath string) (iniPath string, tcDir string, err error) {
	if flagPath == "" {
		iniPath, tcDir = config.FindDefaultIniPath("")
		if iniPath == "" {
			return "", "", fmt.Errorf("could not automatically locate wincmd.ini via Registry or AppData; point TotalPlug at it (Settings > Total Commander, or --ini <path> on the command line)")
		}
		if tcDir == "" {
			// The AppData fallback knows the ini but not the install, and the ini
			// itself does: TC keeps its own folder in [Configuration] InstallDir.
			tcDir = config.ResolveTCDir(iniPath)
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

	// Which install does this ini describe? Only the file system around it can
	// answer: an ini copied out of a portable Total Commander still names the
	// folder it came from, and following that claim scans a Total Commander the
	// user never selected. An unresolved directory is reported as such - the
	// %COMMANDER_PATH% placeholder then stays literal, which reads as "plugin not
	// found" instead of silently matching some other install's plugins.
	tcDir = config.ResolveTCDir(abs)
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
