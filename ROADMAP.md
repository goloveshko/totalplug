# TotalPlug Roadmap

Progress and plan tracker for the Total Commander plugin updater.
One page, three sections: what already works, what the GUI phase will add,
and accepted debt we deliberately postponed.

## Shipped

### Core engine
- Total Commander auto-discovery: registry key `SOFTWARE\Ghisler\Total Commander`,
  `%APPDATA%\GHISLER` fallback, portable install detection (`wincmd.exe` next to `wincmd.ini`).
- `wincmd.ini` parsing for all four plugin sections (Packer / Lister / FileSystem / Content)
  with dedup by path and `%COMMANDER_PATH%` / `%COMMANDER_DRIVE%` expansion.
- Version extraction tiers: PE version resource → companion DLLs → readme/INF regex → `Unknown`.
- Community catalog from GitHub Pages CDN with 24 h disk cache and offline
  expired-cache fallback (`%APPDATA%\TotalPlug\community_catalog_cache.json`).
- Per-plugin update source override: `github:Owner/Repo` custom source via Releases API.
- Strict version comparison; unresolvable versions classify as `StatusUncertain`
  and are never auto-offered as updates.
- Shared scan pipeline (`internal/pipeline`) used by both CLI and GUI:
  resolve ini → parse → enrich versions → apply overrides → check updates.

### Safe updates
- Download + extract to temp first, side-by-side copy backup
  (`<plugin-dir>.bak_<unix>`), then apply over the live directory.
- Automatic rollback of exactly the files touched before a failure;
  backup removed after a successful rollback, kept for manual recovery otherwise.
- Windows file-lock detection (error codes 5/32/33) with a
  "close Total Commander and run again" hint.
- Downloads restricted to https (loopback http allowed for tests) and capped at 256 MiB.
- Zip-slip guarded extraction.

### CLI (`bin/totalplug.exe`)
- Scan report with update summary table.
- `--refresh-cache / -r`, `--ini <path> / -c` (portable installs),
  `--update / -u`, `--update-all`.
- Interactive confirmed updates; per-item `y/N/a` prompts for "blind" updates
  (installed version unknown); `--update-all` never touches blind updates.
- Non-zero exit code when any update fails.

### GUI (Wails, `build/bin/totalplug.exe`)
- Interactive plugin dashboard with scan, filter and per-plugin update
  (commit `54aa8f4`); pre-existing Russian UI text.

### Engineering
- GitHub Actions CI on Windows: `go mod verify`, vet, `go test -race -cover ./...`, CLI build.
- Hermetic unit tests for checker, config, CLI arg/prompt logic, pipeline ini resolution,
  storage and updater (httptest + in-memory zips).
- Corrupt storage JSON is quarantined to `<file>.corrupt` and reported, not silently dropped.

## Next: GUI phase

1. **Cache-first startup scan** — render from disk cache immediately, refresh
   the catalog in the background and emit an update event; no network wait on launch.
2. **Version editor dialog** — for `StatusUncertain` plugins let the user set the
   installed version and/or a custom source; persist via `SetUserOverride`
   (already works and is covered by tests).
3. **Lock-aware updates in GUI** — reuse `ErrFileLocked`; show the "close TC" hint
   in-app instead of a console line; surface the backup path after a successful update.
4. **Remember versions after blind updates** — write an update `Record`
   (`storage.Database` records map currently has no writer) so a manually confirmed
   update is not asked about again.
5. **i18n** — English/Russian locale switch for the frontend (TUI stays English).
6. Keep Wails binding signatures stable (`ScanPlugins(bool)`, `UpdatePlugin(domain.Plugin)`)
   to avoid binding regeneration churn.

## Backlog / accepted debt

- **Unstable plugin IDs** — IDs are derived as `<type>_<folder-name>` and break when a
  plugin moves or is renamed; needs a stable identity scheme before update history
  and overrides can be fully trusted.
- **GitHub releases TTL cache** — unauthenticated API is 60 req/h; add a small TTL
  cache if per-plugin github sources become popular.
- **Backup prune** — `.bak_*` directories accumulate after successful updates;
  add "keep last N per plugin" cleanup.
- **`FileHash` verification** — field exists on records but nothing computes or checks it.
- **`--json` CLI output** and a documented exit-code contract for scripting.
- **Total Commander integration docs** — README section on wiring a "check for updates"
  user-menu button to the CLI.
- **`domain.Config`** struct is dead code (TCPath/IniPath/Language/AutoCheck unwired);
  either wire it into persistence or delete it.
- **Minor `internal/config` debt** — `%COMMANDER_PATH%` expands only the first occurrence;
  no-op `existing` branch in dedup.
