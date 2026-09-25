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
- Community catalog from the totalplug-catalog Pages site (`catalog.min.json` on
  sergey.is-a.dev) with 24 h disk cache and offline
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

## GUI phase — items 1–5 shipped, backlog below remains

1. ~~**Cache-first startup scan**~~ — done (`cef9df4`): `ScanPlugins` answers instantly from the
   disk cache; a single-flight background refresh pushes rows via `plugins:refreshed` events.
2. ~~**Version editor dialog**~~ — done (`ddf59bc`): per-plugin "✎ версия" dialog sets the
   installed version and/or a custom source via `SaveOverride`; empty fields reset the override.
3. ~~**Lock-aware updates in GUI**~~ — done (`b5d51e2`): `UpdatePlugin`
   returns `domain.UpdateResult{locked, backup_path}`; locked targets show an in-app
   "close Total Commander" hint and success reports the kept backup directory.
4. ~~**Remember versions after blind updates**~~ — done (`46c562b`): `Database.RememberVersion`
   is the first writer for `installed_plugins.json`; CLI and GUI record the applied version
   after every successful update. ~~(A full update-history view/API is still open.)~~
   Closed by `8fab018`: append-only `update_history.json` (capped at 200) + GUI history dialog.
   Fixed in `e4a6701`: a successful update now clears a stale manual `CustomVersion`
   override (source/name kept), so the remembered version is no longer shadowed on rescan.
5. ~~**i18n**~~ — done: `frontend/src/i18n.js` dictionaries (EN default, RU toggle in the
   header, choice persisted in localStorage); UI rebuilds in place without reload.
   The TUI stays English-only for now.
6. Binding contract note: `ScanPlugins(bool)` stable; `UpdatePlugin(domain.Plugin)` now
   returns `domain.UpdateResult` (bindings regenerated in-repo) — prefer additive changes.

## GUI redesign (MQ Studio style) — all three stages shipped

1. ~~**Themes + settings**~~ — done (`be2f6b0`, refined in `5078942`): light/dark/system via
   `<html data-theme>` CSS tokens; settings is a full-window view with a «‹ Back» button
   (MQ Studio style), language is a dropdown for future locales; theme/font-scale/language
   persist in localStorage. Error/confirm/history dialogs stay centered static modals —
   dragging was glitchy and removed by user request.
2. ~~**Toolbar + plugin card**~~ — done (`421b995`): compact toolbar (title, search filter,
   history/settings/refresh/scan buttons, all nowrap) with a single ellipsized status line
   below it — the header no longer jumps when scan messages change length;
   row click opens the plugin card (details + embedded version/source editor),
   replacing the per-row "✎ версия" button. The card is the mount point for the
   future install-from-catalog flow ("Установить", catalog search and filters).
   GitHub quick link and true titlebar integration deferred to stage 3 (frameless).
3. ~~**Frameless window with Mica/Acrylic**~~ — done (`6566d60`): frameless Wails window
   with a frontend-drawn titlebar (drag region, double-click maximize, caption buttons).
   Refined in `cbc90ff`: search box and history/refresh/scan/settings actions moved
   into the titlebar as icon buttons (MQ Studio style); fixed-height layout so the
   titlebar never scrolls — views scroll internally.
   Fixed in `b5b0f2c`: action icons now actually hide in the settings view (their
   `display:flex` overrode the UA `[hidden]` rule); scan icon became a bolt (the
   magnifier read as search), focus ring softened to accent, «Back» demoted to a
   quiet ghost button.
   Polished in `e27cace`/`5843297`: app dialog lost its "TotalPlug" title bar for a
   corner ✕ (clear of the text); update errors lead with a localized friendly sentence,
   a named "Open plugin page" link opens in the system browser, and the raw Go error
   sits collapsed under «Technical details»; settings stay full-width with only the
   «Settings» heading centered.
   Mica/Acrylic reverted in `8681d38`: the translucent window washed out the themed UI
   (dark theme + Acrylic showed a semi-transparent titlebar) and the backdrop cannot be
   changed without recreating the window. The window is now always solid; the effect
   dropdown, `html[data-effect]` styling and i18n keys are removed. `window_effect.go`
   and the Get/SetWindowEffect bindings stay as dormant plumbing for a future
   full effects/theme pass. See backlog.
4. **Update progress dialog** — done (`981efbb`): «Обновить» now opens a dialog right
   after the confirm that streams `update:progress` events from the updater
   (download percent via a counting reader over the response body, then extract and
   install phases, indeterminate bar when ContentLength is unknown) and shows the
   download source URL; the final result (success + backup path, locked warning, or
   friendly error with technical details) replaces the progress in the same dialog.
   Not dismissable while running (Esc blocked, OK/✕ hidden until the end).

## Backlog / accepted debt

- **Mica/Acrylic window effect** — temporarily disabled (see stage 3). Re-enabling needs
  a proper theme pass: panels readable over any desktop backdrop, opaque titlebar,
  solid fallback — with `window_effect.txt` + Get/SetWindowEffect bindings already in place.
- ~~**Unstable plugin IDs** — IDs are derived as `<type>_<folder-name>` and break when a
  plugin moves or is renamed; needs a stable identity scheme before update history
  and overrides can be fully trusted.~~ Shipped in `d32a2a1`: IDs are now
  `<TYPE>:<TC-relative lowercased path>` with a one-time rekey of legacy
  records/overrides. Update history can now be built on stable identity.
- **GitHub releases TTL cache** — unauthenticated API is 60 req/h; add a small TTL
  cache if per-plugin github sources become popular.
- ~~**Backup prune** — `.bak_*` directories accumulate after successful updates;
  add "keep last N per plugin" cleanup.~~ Shipped in `3cb2a96` (keep newest 3 per
  plugin, pruned best-effort after each successful update).
- **`FileHash` verification** — field exists on records but nothing computes or checks it.
- ~~**Non-ZIP plugin archives** — totalcmd.net serves many plugins as RAR; since `b332687`
  they are rejected with a clear "unsupported archive format" hint instead of a cryptic
  zip error. Supporting RAR/7z extraction would make those updates work.~~
  Shipped in `1102346`: RAR (nwaples/rardecode) and 7z (bodgit/sevenzip) extract alongside
  ZIP, dispatching on the magic-byte format; gzip/unknown still rejected.
- **`--json` CLI output** and a documented exit-code contract for scripting.
- **Total Commander integration docs** — README section on wiring a "check for updates"
  user-menu button to the CLI.
- **`domain.Config`** struct is dead code (TCPath/IniPath/Language/AutoCheck unwired);
  either wire it into persistence or delete it.
- **Minor `internal/config` debt** — `%COMMANDER_PATH%` expands only the first occurrence;
  no-op `existing` branch in dedup.
