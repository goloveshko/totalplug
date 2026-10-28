# TotalPlug Roadmap

Progress and plan tracker for the Total Commander plugin updater.
One page: what already works, what the GUI phase added, the candidates for the
next release (v2), and the accepted debt we deliberately postponed.

## Shipped

### Core engine

- Total Commander auto-discovery: registry key `SOFTWARE\Ghisler\Total Commander`,
  `%APPDATA%\GHISLER` fallback, portable install detection (`wincmd.exe` next to `wincmd.ini`).
  The AppData fallback answers with an _empty_ install dir: that ini is the roaming
  settings copy, so its own folder is `%APPDATA%`, and reporting that as the install
  dir used to expand `%COMMANDER_PATH%` into a directory no plugin lives in. Unresolved
  leaves the placeholder in the path, which is then reported as not found, and the CLI
  says the dir is unresolved instead of printing `%APPDATA%`.
- `wincmd.ini` parsing for all four plugin sections (Packer / Lister / FileSystem / Content)
  with dedup by path and `%COMMANDER_PATH%` / `%COMMANDER_DRIVE%` expansion.
- Version extraction tiers: PE version resource → companion DLLs → readme/INF regex → `Unknown`.
- Community catalog from the totalplug-catalog Pages site (`catalog.min.json` on
  sergey.is-a.dev), revalidated by ETag on every launch (an unchanged catalog
  answers with a 304 instead of the full download) and served from the on-disk
  copy when offline (`cache\community_catalog.json` in the data folder).
- Per-plugin update source override: `github:Owner/Repo` custom source via Releases API.
- Strict version comparison; unresolvable versions classify as `StatusUncertain`
  and are never auto-offered as updates.
- Shared scan pipeline (`internal/pipeline`) used by both CLI and GUI:
  resolve ini → parse → enrich versions → apply overrides → check updates.

### Safe updates

- Download + extract to temp first, then a copy backup of the plugin directory into
  the data folder (`<data>\backups\<type folder>\<plugin>.bak_<unix>`) and the apply
  over the live directory. Total Commander's tree is not written to.
- Automatic rollback of exactly the files touched before a failure;
  backup removed after a successful rollback, kept for manual recovery otherwise.
- Retention from `settings.json`, read by CLI and GUI alike: the newest N copies per
  plugin are never deleted, and past them a copy goes once older than the day limit
  (0 days deletes nothing). Defaults: 3 copies, 30 days.
- Windows file-lock detection (error codes 5/32/33) with a
  "close Total Commander and run again" hint.
- Downloads default to https (loopback http allowed for tests) and are capped at 256 MiB.
  The GUI can retry a plain-HTTP source with explicit per-plugin consent; the CLI stays strict.
- Mirror fallback, in two layers. The catalog-declared one (`7c6b18d`): the checker collects
  ordered download URLs per plugin from the catalog's `available_sources` (resolved first, then
  same-version mirrors, stable order) and the updater tries each until one delivers a usable
  archive. In the published catalog only 3 of 1213 entries carry `available_sources` at all, and
  each holds what the _other_ resolver returned rather than a second host for the same version.
  Giving the catalog a mirror field is decided against (2026-10-20): publishing ~1200 mirrors
  would mean scraping ~1200 plugin pages on every build, so this layer stays dormant by design
  and the search below is the mechanism.
  The lazy layer (`5c2ae8c`, adapted to the site redesign in `607f048`): once every known
  source has failed, the updater reads the current totalcmd.net plugin page and its linked
  Russian counterpart. It verifies the counterpart advertises the exact requested release,
  then resolves its main download; both HEAD and archive GET redirects must stay on trusted
  HTTPS hosts (`wincmd.ru` and its subdomains). Both old `download.php?id=` and current
  `/download/<id>/main` source URLs are recognised. No catalog change or bulk scraping is
  needed: only broken downloads fetch pages. Missing markup, a different release or an
  untrusted redirect degrades to the ordinary error naming the plugin page.
- Zip-slip guarded extraction.
- Total decompressed output of one archive capped at 1 GiB (`DefaultMaxExtractBytes`,
  one `extractLimiter` shared by the ZIP/RAR/7z loops in `internal/updater`), so a small
  crafted bomb is stopped before it fills the disk — the 256 MiB wire cap alone cannot
  catch it because the archive stays tiny until it is unpacked (`b1bb4aa`).
- Files are copied into place by rename, not by truncating the target: `copyFile` writes a
  sibling temp and renames it over the destination. An in-place write that broke halfway
  left a damaged file `applyTree` had not recorded, so rollback skipped it and the backup
  holding the only good copy was then deleted as a success. The rename is also where a
  target another process holds fails with errno 5, which is what lock detection keys on:
  a file Total Commander merely has open renames through (delete-pending), while one it
  has _loaded_ as an image does not, so an update or a rollback of a plugin the session
  is using is the case the "close Total Commander" hint is for.
- An update is refused when the plugin file does not live in a folder of its own
  (`ErrPluginNeedsOwnFolder`), because both the backup and the unpack take the whole parent
  directory: a `wincmd.ini` entry pointing straight at a file next to `wincmd.exe` /
  `totalcmd64.exe`, or into a folder Total Commander shares between every plugin of a type
  (`plugins`, `wcx`, `wlx`, `wfx`, `wdx` — `config.IsGenericPluginDir`, the list
  `deriveRealPluginName` already used for naming), would have replaced everything beside it.
- `App.UpdatePlugin` holds a mutex for its whole run. The frontend already awaits updates in
  order, but a Wails binding is callable from anywhere, and two runs would fight over the one
  progress listener and over this process' single scratch folder, where the second run empties
  the package the first is still unpacking.
- A package that unpacked is kept in that scratch (`downloaded/`, named after the URL that
  produced it) until the apply goes through or the run ends, and the updater looks there before
  it touches the network: the retry a user asks for after closing Total Commander starts from
  the bytes this process already downloaded and proved rather than fetching them a second time.
  One that failed to unpack is dropped instead of kept — magic bytes alone are not proof, and a
  retry has to stay free to ask for another copy.
- A rollback from the update history merges the kept copy over the plugin folder instead of
  emptying the folder first, so the files it can replace are replaced while Total Commander
  runs — an update never needed the program closed, and deleting the folder around a live
  image section was the only reason a rollback did. A plugin the session has loaded stays
  in the way of the one file it holds, which is the locked case. What the update brought on
  top of the copy is then taken out, but by the names the line recorded
  (`UpdateEntry.AppliedFiles`, collected by `applyTree`):
  a plugin saves its own settings beside its DLL and the folder can hold a file the user put
  there, so nothing else is touched. A line written before names were recorded removes nothing,
  a name the copy also carries belongs to the older version, and a name leading out of the
  folder is not this tool's to delete.
- A rollback whose copy could not be written restores the folder to the version it had and
  removes the snapshot it had made for itself: the history line still describes the change
  that is in place, so the line stays actionable and the failed run leaves no copy nobody
  points at. `copyTree` returns the path it stopped at precisely for this comparison — both
  passes give up at the same file in the usual case, since the file that blocked the copy
  out was never changed and everything before it is put back. A file the attempt brought
  into the folder is in neither pass's list of names to put back, so after them any file
  the kept copy holds and the snapshot does not is removed as this run's own work; one that
  will not go is named in the failure. Only a run that could not restore the folder either
  keeps that snapshot and says so, because it is then the only readable copy of what was there.

### CLI (`bin/totalplug-cli.exe`, `scripts/build.cmd` / `release.cmd`)

- Scan report with update summary table.
- `--refresh-cache / -r`, `--ini <path> / -c` (portable installs),
  `--update / -u`, `--update-all`, `--version / -v` (the stamped build version).
- Interactive confirmed updates; per-item `y/N/a` prompts for "blind" updates
  (installed version unknown); `--update-all` never touches blind updates.
- Non-zero exit code when any update fails.
- `--history` lists the log newest first and marks the line that can still go
  back; `--rollback <id or name>` puts that line's copy back after one `y/N`.
  Neither scans nor goes online. The rule about which line may be rolled back is
  shared with the GUI panel (`internal/rollback`), so the two cannot drift.

### GUI (Wails, `bin/totalplug.exe`, built by `scripts/build-gui.cmd`)

- Interactive plugin dashboard with scan, filter and per-plugin update
  (commit `54aa8f4`); pre-existing Russian UI text.
- Since then: light/dark/system themes, EN/RU interface, plugin card with the
  override editor, update history, batch «Update all», an About panel and the
  self-update check — see the redesign and v1 sections below.
- Settings › Total Commander: the panel names the `wincmd.ini` the next scan
  opens, accepts a manually picked one (native picker or a typed path) for a
  Total Commander the registry does not know about, and hands the choice back to
  auto-detection. The path is stored in `settings.json`, so a portable install
  carries it with the rest of the data, and both scan call sites read it.
- The update history is grouped by plugin. A group leads with its newest change,
  which is the only line a copy is kept for, so folding the earlier ones away
  behind a +/− in the name cell hides nothing you could act on. The dialog opens
  at the top of the log: Close is the last control inside a `<dialog>` that
  scrolls, and the plain `focus()` handed the log its own end.

### Engineering

- GitHub Actions CI: Windows job (`npm ci` + `npm run build` first, because the root
  `main.go` embeds `frontend/dist` and a fresh checkout has none, then `go mod verify`,
  vet, `go test -race -cover ./...`, CLI build) plus an ubuntu job that checks the
  formatting and builds the frontend on its own (`f6702e3`, `e201d1a`). Both jobs have
  been green on every push to GitHub since the first one (`59618d3`), the formatting
  gate included, so the workflow is no longer the untested layer it was while `origin`
  pointed at a local Gitea.
- Release packaging in CI (`.github/workflows/release.yml`): a `v*` tag starts a Windows
  job that installs the Wails CLI at the version `go.mod` pins and runs
  `scripts/build.ps1 package`, so the artifact keeps coming from the one script a hand
  build uses — payload, archive name and the `.sha256` line included; the actions carry
  the same pins as `ci.yml`. `fetch-depth: 0` is load-bearing: a shallow checkout has no
  tags, `git describe` fails, and the script silently stamps the `wails.json` fallback.
  Before uploading, the job reads both stamps back (CLI `--version`, the GUI's
  `FileVersion`) and fails otherwise — that is the failure worth a gate, because the
  fallback would publish a `1.0.0` archive under a new tag. It then opens a **draft**
  release with the zip and its checksum: the notes are bilingual and pasted by hand, and
  publishing is what moves `releases/latest`. The first target was the v1.1 tag; this roadmap
  does not yet record a verified remote run.
- Prettier over the whole tree, not just the frontend: `.prettierrc` keeps 4 spaces and
  single quotes for JS/CSS/HTML/Markdown (how `frontend/src` is written by hand) and
  overrides JSON/YAML back to 2 spaces and double quotes (how npm, Wails and a pasted
  workflow example write them), so `wails.json` and `package.json` come out of the next
  tool run already formatted instead of dirty. `npm run format:check` is the gate, wired
  into that ubuntu job. One thing to know before pointing `--write` at prose: an inline
  code span wrapped across two lines makes prettier push its continuation to column 0,
  and a line that then begins with `+ ` parses as a nested list — so such a span has to
  stay on one line.
- Build scripts: `scripts/build-gui.cmd` (daily GUI, no cleaning) and
  `scripts/release-gui.cmd` (`-trimpath`, previous binary removed by name); both
  hard link Wails' output into `bin/totalplug.exe`, next to `bin/totalplug-cli.exe`,
  so `bin/` is the single artifact folder — and so a GUI window left open no longer
  blocks publishing, which the copy step did by writing over the running image.
  Wails' own `-clean` is deliberately unused:
  it deletes `build/bin` as its first step, and a handle on that directory (IDE or
  Defender watcher, a TC/Explorer tab, a shell cwd) then fails the build _after_
  the old binary is already gone.
- Hermetic unit tests for checker, config, CLI arg/prompt logic, pipeline ini resolution,
  storage and updater (httptest + in-memory zips).
- Clean-environment run: `scripts/sandbox.cmd` (the `sandbox` verb of `scripts/build.ps1`)
  stages a release build and a Total Commander fixture in `bin/totalplug-sandbox`,
  generates the `.wsb` and starts Windows Sandbox. The fixture is the layout a 64-bit
  portable install uses — `<install>\ini\wincmd.ini`, no `wincmd.exe` anywhere — which is
  the case that used to resolve to the wrong install. The image has no WebView2 runtime,
  so the Evergreen Standalone Installer is downloaded on the host once and installed at
  logon from the shared folder; the bootstrapper Wails' own prompt offers would otherwise
  re-download it into every fresh VM. `webview2-install.log` next to the build is the
  evidence the install took, and `-NoRuntime` skips the whole thing to reproduce the
  missing-runtime first run.
- Corrupt storage JSON is quarantined to `<file>.corrupt` and reported, not silently dropped.
- Hardening pass from the 1.2 audit (2026-10-24): the version comparator stops at the
  first non-numeric character, so a `2.0 x64` stamp reads as 2.0 and no longer beats
  2.1 (digits after a letter used to merge into the previous component); the scanner
  clamps the PE translation-array count, so a resource claiming more than 64K entries
  panics nothing; `totalcmd.exe` joined the launcher list and the list moved to
  `config.TCLaunchers`, shared by install-dir detection and the updater's root
  bounding (a root one of them did not know made the updater refuse a valid
  rollback); settings writes take a mutex — binding goroutines and OnBeforeClose all
  staged through the same `.tmp`; the checker's HTTP clients re-check the scheme on
  every redirect (the https→http downgrade the updater already refused); the GitHub
  release cache and the release-asset pick (first archive-named asset) are fixed; a
  `github:` override that resolves to nothing stays unresolved with its repo link
  instead of silently re-matching against the catalog; a 304 against an unusable
  cache says so instead of "HTTP error: 304"; wincmd.ini splits `flags,path` only
  when the part before the comma is a number, so a comma inside a path survives.
- Hardening pass from the first public issue (2026-10-28), catalog and versions: the match
  tells the two builds of a plugin apart — the loose name match dropped the bit marker, so a
  64-bit plugin answered to the 32-bit entry and the update unpacked the other build's
  archive over it (the update history on this machine shows exactly that) — and it walks the
  catalog by tier now, keeping the marker, letting the newest release win among the entries
  that match, skipping an entry whose architecture cannot serve the installed build, and
  matching nothing when a name normalizes to nothing (a Russian-named plugin used to answer
  to the first Cyrillic entry); the version a plugin declares in its own `pluginst.inf` is
  read as a key (`version=1.2`) and trusted like a binary version — the text rule wanted
  whitespace after the word, so the key every plugin writes was invisible — while a version
  read out of prose no longer authorizes an update; a catalog that cannot be reached marks
  the rows uncertain instead of handing on the ini's "up to date".
- Hardening pass from the first public issue (2026-10-28), files and window: every writer
  takes a temp name of its own, the run folder is named after the process and not after a
  pid (Windows reuses pids, and the stale archive of an earlier run was installed instead of
  the current one), two copies of a plugin in the same second stay apart, and a named mutex
  keeps the window and the command line from writing the same data folder at once; a data
  file holding `null` no longer panics the window; the command line prints a help, answers a
  script with an exit code (3 for a Total Commander it did not find) and says so when an
  update went through but could not be logged; the page's content security policy keeps the
  inline style attribute the progress bar needs, the first-run dialog says where the ini is,
  and the table takes the width of the window instead of a centred measure.

## GUI phase — items 1–5 shipped; open work now sits under Next release / Accepted debt

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

## GUI redesign (MQ Studio style) — stages shipped, polish ongoing

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
   full effects/theme pass. Re-enabled in stage 18 with the missing theme sync that
   caused the wash-out.
4. **Update progress dialog** — done (`981efbb`): «Обновить» now opens a dialog right
   after the confirm that streams `update:progress` events from the updater
   (download percent via a counting reader over the response body, then extract and
   install phases, indeterminate bar when ContentLength is unknown) and shows the
   download source URL; the final result (success + backup path, locked warning, or
   friendly error with technical details) replaces the progress in the same dialog.
   Not dismissable while running (Esc blocked, OK/✕ hidden until the end).
   Superseded in stage 18: progress and single-plugin outcomes moved into the row;
   the dialog now carries only errors, locks and the batch summary.
5. **Catalog freshness dot** — done (`6b50114`): the refresh icon carries a colored
   dot instead of status-bar wording — amber while the table shows cached catalog
   data, green when the background update lands, red when it fails; the tooltip
   gets the matching localized phrase and the state survives the language-switch
   DOM rebuild.
6. **Plugin counter chip** — done (`c9906b6`, labeled in `03c0b71`): a muted pill
   next to the TotalPlug titlebar title shows «плагинов N» / "plugins N"; while
   the search filter hides rows it switches to `shown/total`, and it disappears
   when nothing is loaded or settings is open. The count lives in module state
   so it survives the language-switch DOM rebuild. The status bar no longer
   repeats the number («Каталог обновлён», «Сканирование завершено»).
7. **Status bar removed** — done (`8cdfeee`): the bar duplicated what the chip
   and the freshness dot already say, so it is gone. Errors (scan failure,
   override save, history load) now open the app dialog with friendly text +
   technical details; a catalog URL in a scan error is labeled «Каталог
   плагинов». Background refresh failures stay silent — the red dot's tooltip
   carries the raw reason.
8. **Plugin card source + homepage** — done (`265d2f1`): the card shows the
   actual update source (catalog `available_sources` key, `github:Owner/Repo`
   override, or —) via new `domain.Plugin.Source`, plus a clickable
   «Домашняя страница» (web_url → BrowserOpenURL). The override editor is
   deliberately NOT pre-filled: a value there shadows the catalog.
   User-side source lists were declined — a mirror is not something to type per plugin, and
   storage is not where one belongs. What was left open, "put them in the catalog repo", is
   declined too by the time the mirror search above shipped: see the Safe updates bullet.
9. **Update source UX** — done (`f1ade2c`): the editor field now accepts a pasted
   GitHub page URL (or bare `owner/repo`) and canonicalizes to
   `github:Owner/Repo` on save; the placeholder says «вставьте страницу
   GitHub». Non-matching text is stored as typed.
10. **GitHub release cache for first paint** — done (`154eb25`): successful
    `github:Owner/Repo` lookups are remembered per repo in
    `cache\github_releases.json` inside the data folder; the cache-only startup
    pass replays the last known version/URLs instead of flickering
    «N/A (Not in catalog)», and the background network pass refreshes both
    the UI and the cache. Best-effort file, same pattern as the catalog cache.
11. **Demo mock backend for design work** — done (`02515f1`): `src/mock.js`
    fakes the Wails bridge (sample plugins, background refresh, update
    progress incl. an always-locked plugin, overrides, history) only under
    `npm run dev`; the full GUI runs in a plain browser with no Go/Wails.
    Workflow documented in `frontend/README.md`.
12. **External review pass** — done: behavior, i18n and theme fixes
    from the design review, verified in the browser harness.
    - i18n (`b35e0d2`): CLDR plurals through `Intl.PluralRules` (`tp('count.plugins', n)` →
      «2 плагина», «5 плагинов»); `setLang` syncs `document.documentElement.lang`
      (was permanently `ru`); `{name}` substitution is `$&`-safe; emoji
      (🔍 🔄 🕘 ⚙ ⚠️) removed from locale strings — icons are SVG in markup,
      tooltips/screen-reader labels are text; «обновлен» → «обновлён»,
      «общественным каталогом» → «каталогом сообщества».
    - Table (`b35e0d2`, `6cec710`): the «Источник» column showed `confidence`, not a source — the column
      is gone (6 columns), detection stays in the plugin card; every cell
      (`name`, `type`, versions, `path`) is escaped; the type cell became a muted
      label (`.cell-type`, gone with the stage 17 headings) instead of `<strong>`; badges use monochrome glyphs
      (`✓ ↑ ✗ ?`); placeholder rows share `.table-empty`; sticky `thead` with the
      scroll container moved to `.table-container` (inset shadow instead of a
      collapsed border); startup state is «Сканирую wincmd.ini…», a failed scan
      shows its own row instead of pretending the table is empty.
    - Behavior (`b35e0d2`): `onclick="handleUpdate(i)"` replaced by the delegated `data-update`
      click (no `window.` global — a future `script-src` CSP stays possible) plus a
      `progressRunning` guard so a second click cannot start a parallel update;
      titlebar double-click ignores inputs/buttons (it used to maximise from the
      search box); history dates format in the UI locale (`ru-RU`/`en-GB`), its
      button reads «Закрыть»; dialogs focus the safe button (Cancel on confirm) and
      return focus on close; theme cards are real `<button>`s with `aria-pressed`
      instead of `div[role=button]` (Enter/Space now work); icon/caption buttons got
      `aria-label`; catalog-state dot text is mirrored into `aria-label`.
    - CSS (`6cec710`): `color-scheme` per theme (native scrollbars, `<select>`, range,
      search-clear), visible progress track (`--track-bg`), hover for dialog ghost
      buttons, `dialog { max-height: 85vh; overflow-y: auto }`, `--badge-err` for
      dark, `.btn-update:hover` via a themed `--badge-avail-strong` token, light
      badges darkened for AA (`--badge-up #15803d`, `--badge-avail #c05621`) with
      `color-mix` tints replacing per-theme rgba,
      `[hidden] { display: none !important }` (the three element-specific patches
      removed), tabular numerals and `user-select: text` for versions/paths,
      `prefers-reduced-motion`, titlebar and button widths in rem so the font-scale
      setting scales them, Windows-only font stack.
    - Shell (`cd81f89`, `1152dce`): `index.html` defaults to `lang="en"` with a pre-paint script that
      resolves theme + language before the module graph loads (no light flash on a
      dark start) and an inline first-paint background color; unused Wails template
      `src/app.css` deleted.
    - Deliberately not done: the CSP meta tag (done later, see stage 13);
      `SetWindowEffect` sync from `setTheme` (moot while the window effect is
      reverted, and it needs a window recreation anyway); column sorting.
13. **Strict CSP for the packaged GUI** — done (`ab10d64`, `28484d3`): `index.html`
    declares `'self'` for every directive but `object-src 'none'` — no
    `'unsafe-inline'`, no `'unsafe-eval'`. To make that possible the pre-paint
    theme/lang script and the first-paint background moved to
    `frontend/public/boot.{js,css}` (Vite copies `public/` verbatim, so
    they load as classic same-origin resources instead of inline tags, and a
    parser-blocking `<script src>` still runs before first paint). The policy ships
    in the production bundle only: the new `vite.config.js` strips the meta tag for
    `npm run dev`, where Vite injects imported CSS as inline `<style>` elements
    (the same config pins the dev port to 5199, which `frontend/README.md` always
    claimed). Wails v2.14 is compatible by construction — it injects
    `/wails/runtime.js` and `/wails/ipc.js` into `<head>` as `<script src>` only,
    uses no `eval`/`new Function`, and delivers Go→JS callbacks through WebView2
    `ExecuteScript`, which a page CSP does not gate. Verified: `wails build`
    succeeds and the binary embeds `boot.*` + the meta tag; under `vite preview`
    (policy active) all four resources load with no violation and both stylesheets
    parse; the dev harness is unchanged. Confirmed on a live
    `build/bin/totalplug.exe` by the user: table, dark theme, plugin card and
    updates all work under the policy.
14. **Card readability pass** — done (`c624e4e`, `c65ee51`, `a98ce65`), from the
    same live run: the stored `github:Owner/Repo` key is a storage detail, so the
    card shows it as a clickable `https://github.com/Owner/Repo` and the override
    editor prefills that same URL form (`displaySource()` is the inverse of
    `normalizeSource()`, so a save round-trips unchanged); long values wrap instead
    of pushing the dialog into a horizontal scrollbar; the dev mock now keeps
    `custom_version`/`custom_source` on the record like the Go bridge does, which is
    what made the prefill checkable without building the exe.
15. **Card reorganization** — done (`92cc67b`), same feedback pass: the read-only
    "Update source" row is gone entirely — for a GitHub plugin it restated the
    homepage, and `totalcmd` is storage vocabulary. The provider that resolved the
    version is now a 14px glyph beside the «В каталоге» value (octicons
    `mark-github-16`, a placeholder grid until the catalog has its own mark;
    `title` + `aria-label` name it), while the two genuinely technical values — the
    raw `plugin.source` key and the install path — collapsed into a «Подробности»
    `<details>` block using the error dialog's disclosure pattern. The one remaining
    link row is «Сайт», whose text drops the `https://` prefix (`shortUrl()`) while
    the `href` keeps it, so `BrowserOpenURL` still receives a usable address.
    Verified in the dev harness across all nine mock rows: correct glyph per source,
    details closed by default and overflow-free open or closed, RU and EN labels
    both fitting the dialog.
16. **One module per screen** — done (`e5ef0c1`): `main.js` had grown to 1305 lines
    holding every screen's markup, state and listeners at once, so following one flow
    meant keeping all of them in head. Each screen is now a module of its own —
    `catalog.js`, `card.js`, `update.js`, `progress.js`, `dialog.js`, `history.js`,
    `about.js`, `tc.js`, `settings.js` — over four shared bases (`state.js`, `dom.js`,
    `text.js`, `plugin.js`), and the file that remains is 134 lines: the titlebar, the
    row click that opens either the card or an update, and the language switch that
    rebuilds every label. Plain ESM, so no new dependency and nothing to relax from the
    stage 13 policy; the production bundle pays 1.67 kB (+3.7%) for the import graph.
    Two findings worth keeping. Only three pieces of state are genuinely shared — the
    plugin list, whether an update run is in progress, and whether settings is open.
    And `EventsOn` must stay at module scope, registered once before the first
    `buildDom()`: a language switch re-binds every screen, so the same event would
    acquire a second listener each time.
17. **Type groups in the table** — done (`8c9d0ad`, `b20ba2b`, `207fe01`): one undivided
    list made the reader compare the type cell row by row, while the scan already hands
    rows over in `wincmd.ini` order — `internal/config/ini.go` parses Packer, Lister,
    FileSystem, Content and nothing re-sorts the result — so the types were runs all
    along, only unlabeled. Each run now carries a heading with its type and count
    («WFX · 2 плагина»), drawn from the column header's own tokens with less padding, and
    the type column is deleted with its `col.type` keys: repeated down a run it read as
    noise once the heading above said the same thing. Two invariants the grouping had to
    keep. A run is keyed `type:id`, because two runs of one type must not collide onto one
    heading, and a row still holds its index into the full plugin list rather than into
    the filtered view, since the card and both update paths resolve a plugin by that index
    — which is also why a heading itself opens nothing. The mock came first because it
    could not show a run at all: it invented lowercase types (`ltx`, `dsx`, `tbr`)
    scattered through the list, and its ids had drifted from the history entry it seeds.
    Deliberately no tint per group — the band and its border are the separation. Verified
    in the dev harness in both languages: four headings with CLDR plurals, indices 0..10
    preserved, the name button and the delegated cell click resolving the same plugin in a
    later group, a filtered view labelling only the surviving run, sticky `thead` above the
    headings, contrast 5.21 dark / 4.82 light, and no horizontal scrollbar at the width the
    six-column table used. A plain `npm run build` reports 103.22 kB JS (gzip 36.42).
18. **UX/UI review pass** — done (`353cd2e`, `61396df`): the 14-point list from
    2026-10-21, implemented whole.
    - Quiet table: up-to-date rows carry no badge, an offer is an accent latest version
      plus one tonal button, and status/action share the last column — badges remain only
      for exceptions. The refresh and scan icons became one «Check for updates» button
      (`#btn-check`, still `runScan(false)`), the freshness dot riding on it. A 1120 px
      content cap, thin themed scrollbars, tabular UI-font numbers, and one accent:
      `#005FB8` light / `#60CDFF` with dark text in dark, disabled fills included.
    - The «N updates» band is a filter toggle (`updatesOnly`) that resets itself when
      nothing is offered; `data-idx` still indexes the full plugin list.
    - Single-plugin updates no longer open the progress dialog: the row streams its own
      bar and states the outcome in place. Dialogs are left to errors and locks, the batch
      confirm/summary, and the card. `ProgressOverlay.svelte`/`progress.js` are deleted,
      and the success message no longer recites the backup path.
    - Card actions: «Update to X» primary, «Change…» secondary, the version/source editor
      only behind the latter; an unknown installed version explains itself and focuses the
      field. Cancel restores focus after a `tick()` — the Change button it returns to is
      removed together with the editor. The card shows the release date; `published_at`
      now travels from the catalog and the GitHub release cache into `domain.Plugin`.
    - Settings: the selected section is a tint plus an inset accent rail instead of a solid
      pill, and uppercase/letter-spacing labels are gone. Ctrl+F focuses search, Esc clears
      it. Settings content shares the table's 1120 px cap.
    - Mica: `internal/gui/backdrop.go` picks the backdrop before the window exists — Mica
      on Windows 11 build 22621+, a solid fallback elsewhere — and `GetWindowEffect` tells
      the page which one it got (`data-backdrop="mica"`). Only the page background goes
      transparent; the table, dialogs and settings panel stay opaque cards. The app theme
      now calls `WindowSetDarkTheme`/`WindowSetLightTheme`, so DWM tints the backdrop to
      match instead of to the system theme — the cause of the 2026-09-25 wash-out. No
      setting toggle: the backdrop still cannot change without recreating the window.
    - Verified in the dev harness in both languages: filter/count, keyboard, card editor
      and focus return, single update, lock and error paths, batch summary incl. technical
      details, theme switch, and Mica transparencies; `gofmt`, `go vet`, `go test`,
      `prettier --check` and `vite build` all pass (103.50 kB JS, gzip 36.88).
19. **Plain-HTTP consent, and a table that holds still** — done and accepted
    (2026-10-22; `74b9f3d`, `61396df`): the two defects the first real run after
    stage 18 turned up.
    - FileInfo exposed the ordinary plaintext-redirect case:
      `https://totalcmd.net/download.php?id=fileinfo` answers 302 to
      `http://st.free.fr/phpmyvisites.php?url=http%3A//fg.tcplugins.free.fr/...`. The strict
      redirect guard refuses that hop; consenting removes the policy refusal, not the dead
      tracking URL. Plaintext consent is the user's decision, not a catalog edit.
      `validateDownloadURL` takes an `allowInsecure` argument that only the `http` branch
      honours — ftp and file stay refused. `UpdatePluginInsecure` carries it through the
      catalog candidate list; discovered mirrors still require trusted HTTPS hosts even
      after consent. A refusal names itself `ErrPlainHTTPSource` next to `ErrInsecureScheme`,
      so the CLI keeps one message and the bridge can tell the retryable case from the rest.
      `App.UpdatePlugin` reports it as `UpdateResult{Insecure: true}`, the channel the lock
      already uses, and the row then asks one question per plugin: Cancel leaves the row
      exactly as it was, OK re-runs that plugin with the opt-in. Asking where the refusal
      happens rather than predicting it from the record is the point — the plaintext hop is
      only visible mid-redirect. In a batch the question arrives inside the queue: the row
      stops animating while the dialog is open, a decline lands among the skipped rows and
      the queue carries on, because allowing one author's server says nothing about the next.
    - The table shifted while an update ran, and the progress bar is the reason: an
      auto-laid column grows to fit its widest cell, and the inline block is wider than the
      button it replaces — at a 1280 px window the status column went 195 → 269 px and took
      the difference off the other three, sliding every row's version numbers and actions.
      The column is now reserved (`#view-main thead th:last-child { width: 170px }`) and
      anchored right, so the buttons sit at the edge of the cell whatever the column is
      worth. Re-measured across all three phases in both languages: nothing moves by more
      than 0 px and «Распаковка архива» fits the reserved width.
    - Known and deliberately open: the consent does not rescue FileInfo itself. The chain
      above ends in a 404 because the `st.free.fr` tracking script is gone, and the author's
      live file is a direct link the override editor cannot express — `CustomSource` only
      takes `github:Owner/Repo`.
    - Measured the same day, after totalcmd.net shipped its redesign: the old `Mirror`
      anchor is absent from the sampled FileInfo and PEViewer pages, so the old parser
      finds no candidate there (the page itself is still reachable — `/plugring/<id>.html`
      301s to `/plugins/<id>`). The sampled pages offer `/download/<id>/main`, and on the
      PEViewer RU copy the host serves the archive itself —
      `https://wincmd.ru/download/PEViewer3/main` lands on
      `https://wincmd.ru/mirror/files/9924391/wlx_peviewer_3.0.18.rar` (https, and
      `wincmd.ru` is already on the trusted-host list).
      FileInfo is the exception: both hosts redirect to the same dead counter, and the site
      still advertises 2.23 while the author has 2.24. The published catalog is unaffected
      for now — 1216 entries, all `download.php?id=`. The new URL format and mirror layout
      are supported by the repair in stage 20 below. Options for a direct-archive override
      or a local release list remain under discussion; neither is decided or built.
    - Verified: `TestInsecureRefusalIsRetryableOnlyForPlainHTTP` and the redirect tests pin
      the sentinel split, `TestDownloadFileInsecureOptInDoesNotRefusePlainHTTP` that the
      opt-in reaches the network instead of refusing; the harness drove both dialogs in EN
      and RU, cancel and accept, the mid-queue decline and the summary row. `gofmt`,
      `go vet`, `go test`, `prettier --check` and `vite build` pass (104.35 kB JS, gzip 37.17).
20. **Review repairs and current-site mirrors** — done (2026-10-22;
    `74b9f3d`, `607f048`, `61396df`; interaction rechecked in the browser and on the live
    window 2026-10-23).
    - A retained permissive download is scoped to that plugin's path and cannot answer a
      strict call, including when an HTTPS URL redirected to HTTP. `errors.Join` keeps the
      HTTP-consent signal alongside later source failures, rather than letting a dead mirror
      overwrite it; a usable HTTPS candidate still succeeds without asking.
    - Dialog requests use a FIFO: a background scan error waits behind an HTTP question
      instead of abandoning the question's promise. Queued requests reuse the native modal,
      and each confirmation focuses Cancel. Escape declines only the current question;
      delayed close events cannot dismiss a new request.
    - The card's open transition and editor flag, not its draft object, drive input focus.
      Native closure synchronizes state before resetting the editor; the displayed record
      follows the live plugin list without overwriting drafts. An updated plugin no longer
      keeps an obsolete update offer in an already open card. Indeterminate row progress
      removes its inline width so the stylesheet's moving bar is visible.
    - The mirror resolver recognises both download URL formats and the current EN/RU page
      structure. It follows only the same-plugin RU counterpart, requires the exact requested
      version, and selects the main download button, not screenshot links. Both HEAD and GET
      redirects stay on trusted HTTPS hosts, even with primary-source HTTP consent. Mirror
      packages have a separate cache namespace so an ordinary source's retained package
      cannot bypass that allowlist. A failed apply still reuses its own verified mirror
      package on retry, and success forgets it.
    - Regression tests cover both HTTP cache shapes, both HTTP/404 candidate orders,
      different mirror versions, untrusted links and redirects, HEAD/GET disagreement,
      cache-policy separation and mirror retries. The unsafe cache cases failed before
      their repairs and pass afterwards. Eleven synthetic GUI test groups passed, including
      an overlapping scan error and a declined HTTP item in the middle of a batch. Live
      PEViewer mirror HEAD resolves over HTTPS to a 200 RAR response. The fixes do not
      recover FileInfo's dead tracking script or add a local release list.
    - Every implementation commit was checked from its archived index tree with `go vet`
      and `go test`; the Mica and GUI snapshots also built their own frontend. One rollback
      cleanup assertion failed once; three focused reruns and a complete rerun passed
      without changing or skipping the test. Its intermittent failure is not diagnosed.
      Final-source checks also passed `go test -race`, a production Go build, formatting
      and the Wails GUI build.
    - Re-checked on 2026-10-23 in the dev harness and, after rebuilding `bin\totalplug.exe`
      from the clean tree, on the live window. The row bar keeps every column to the same
      width through download, extract and install; the indeterminate phase drops its inline
      width so the stylesheet's moving bar paints; a consent question silences its row;
      declining leaves the offer and the queue intact, accepting patches the row in place;
      three queued requests (consent, scan error, lock notice) each kept their promise; the
      updates filter resets itself to the full list once nothing is left to update, and the
      drafts survive a language switch. The compiler logged no warnings. This harness never
      dispatches a dialog `close` event, so the card's focus return and keyboard activation
      were confirmed on the live window, where the Mica backdrop also tracks the desktop.

## Before the first public release (v1) — shipped

1. **Version baked into the binary** — done (`af81b2e`): `internal/buildinfo.Version`
   is stamped by `scripts/build.ps1` from `git describe --tags --dirty` through
   `-ldflags -X` in all four build paths (dev/release × CLI/GUI), so a plain
   `go build` keeps `"dev"`. The Windows-properties copy is stamped from the same
   value: `wails build` renders VERSIONINFO only from `wails.json` →
   `info.productVersion` and has no alternate-config flag, so the GUI profiles
   write the stamped version into that file, build, and put the file back
   whichever way the build ends — the tracked `1.0.0` is only the fallback for a
   bare `wails dev`. The stock `build/windows/info.json` never declared a
   `FileVersion` string, so that copy did not merely lag: `FileVersionInfo` read
   empty for every field. Adding the string made the resource readable, and an
   unstamped build now reports `dev` about itself instead of claiming `1.0.0`.
2. **Self-update check, no self-replacement** — done (`ee9a617`):
   `checker.CheckAppRelease` reuses the plugin release client
   (`FetchLatestGitHubRelease`, now also returning `html_url`) behind a
   `GitHubAPIBase` test seam, and compares with `CompareVersions`. A `"dev"`
   build reports `comparable: false` and can never claim an update. TotalPlug
   does not swap its own executable: the GUI hands the user the release page URL
   from the API instead of guessing a tag slug.
3. **About panel** — done (`e7344c0`): the place the app talks about itself —
   version chip, the data directory in use and the Go version behind a
   collapsed disclosure, catalog/releases links, a privacy line, plus the update
   row (button label never changes; it only disables while checking, and the
   checked-at time follows the UI language). Errors land in the app dialog with
   the raw message collapsed, like everywhere else.
4. **«Update all» queue** — done (`f961320`): a strip above the table that exists
   only while updates are offerable, one confirm naming the whole queue, then one
   progress dialog walking it strictly sequentially (parallel `UpdatePlugin` calls
   would race on `.bak_*` names and the bar would lie). Rows without a resolved
   download source stay a per-plugin decision, matching CLI `--update-all`; the
   first locked result ends the run, because a lock means Total Commander is
   running. The summary is honest about a partial batch: N updated plus one row
   per success, failure and skip, raw errors collapsed, rescan after any success.
   Verified in the dev harness with always-failing and always-locked mock stubs.
5. Deliberately not in v1: installing plugins from the catalog (next release), an
   NSIS installer or embedded WebView2 runtime, a startup update banner (the check
   stays manual, from the About panel), and a web landing page.
6. **Final pre-release review pass** — a read-through of the code and the docs against
   what the shipped binary actually does:
    - `«Update all»` rebuilt its queue from records captured before the confirm was
      answered, so a `plugins:refreshed` landing in that window had the batch download
      from superseded `download_url`s; it now re-resolves each entry by stable id after
      the confirm, the hazard `handleUpdate` was already fixed for, and drops whatever
      is no longer offerable.
    - `escapeHtml` did not escape `'`, which is a quote the attribute values it feeds
      are delimited by; the progress dialog announced its phase on every download tick
      because the live region was rewritten with the value it already had, so the percent
      moved to its own `aria-hidden` span and only the phase name is announced.
    - Docs corrected where they had drifted: `frontend/README.md` translated to English
      and its Mica claim dropped (the effect is disabled), `docs/release.md` now says
      `-race` needs `CGO_ENABLED=1` and a C compiler, README's privacy paragraph names
      the throwaway download folder and its `%TEMP%\totalplug` lifecycle, and the
      four-part build stamp is explained where `--version` is.
    - The personal absolute path in a commented-out `replace` directive is gone from
      `go.mod`, and `domain.Override.IgnoreUpdates` — declared, persisted and read by
      nothing — is deleted along with its generated TypeScript mirror.

## Next release (v2)

- **Install plugins from the catalog** — v1 updates only plugins `wincmd.ini` already
  references; adding one is the reverse pass: pick a catalog entry, download it,
  extract it into its own folder under the plugins root and write the matching ini
  section. This is the "next release" the v1 scope notes point at.
- ~~**Sorting inside a type group** (at least status and name)~~ Shipped before v2
  (2026-10-24): the three content columns sort within each type group — name,
  installed version, catalog version (version-aware, the same leading-numeric-core
  rule the Go checker compares by, `frontend/src/plugin.js` `compareVersions`) —
  with an ascending → descending → off cycle, `aria-sort` on the header, and F5
  wired as the rescan shortcut. The search box also matches the type and the
  folder now, not only the name, through one shared predicate the count chip
  uses too (`matchesQuery`).
- **`--json` CLI output** and a documented exit-code contract for scripting.
- **Total Commander integration docs** — README section on wiring a "check for updates"
  user-menu button to the CLI.

## Accepted debt / as needed

- ~~**Mica/Acrylic window effect** — temporarily disabled (see stage 3).~~ Shipped in
  stage 18: Mica only (no Acrylic), Windows 11 22H2+ with a solid fallback, opaque
  panels over the backdrop, and DWM theme sync. The no-op `window_effect` setting and
  setter are gone; `GetWindowEffect` reports the backdrop selected at startup.
- ~~**Unstable plugin IDs** — IDs are derived as `<type>_<folder-name>` and break when a
  plugin moves or is renamed; needs a stable identity scheme before update history
  and overrides can be fully trusted.~~ Shipped in `d32a2a1`: IDs are now
  `<TYPE>:<TC-relative lowercased path>` with a one-time rekey of legacy
  records/overrides. Update history can now be built on stable identity.
- **GitHub releases TTL cache** — unauthenticated API is 60 req/h; add a small TTL
  cache if per-plugin github sources become popular.
- ~~**Backup prune** — `.bak_*` directories accumulate after successful updates;
  add "keep last N per plugin" cleanup.~~ Shipped in `3cb2a96` (keep newest 3 per
  plugin, pruned best-effort after each successful update). Now a policy in the
  settings file — count plus age — and the copies live in the data folder instead of
  the Total Commander tree, with the old siblings carried over when a plugin updates.
- ~~**Backup in the update history** — `UpdateEntry` stores no `BackupPath`, so the
  history dialog cannot show which copy belongs to which applied update, and there is
  no way to delete one from there.~~ Shipped: a line now names the folder it changed,
  the copy it kept and the version that copy held, so it can put that copy back — the
  first destructive action in the GUI, confirmed in the row it is about instead of in a
  second dialog over this one. Only the newest line about a plugin offers it, and only
  when that line was an update: rolling a rollback back is what the update button does,
  and an older line no longer describes the folder. A copy retention already deleted
  stops offering it, and lines written by older versions have nothing to roll back to.
  The delete button was dropped: with a rollback there is nothing left to clean by hand.
- ~~**`FileHash` verification** — field exists on records but nothing computes or
  checks it.~~ Resolved by removal (2026-10-24): the field was never written or read,
  and a hash of the _installed_ folder is the wrong shape for the need behind it —
  the updater's lock-aware rename and the backup it keeps are what guard an apply.
  Records written by older versions with a `file_hash` key simply ignore it.
- ~~**Non-ZIP plugin archives** — totalcmd.net serves many plugins as RAR; since `b332687`
  they are rejected with a clear "unsupported archive format" hint instead of a cryptic
  zip error. Supporting RAR/7z extraction would make those updates work.~~
  Shipped in `1102346`: RAR (nwaples/rardecode) and 7z (bodgit/sevenzip) extract alongside
  ZIP, dispatching on the magic-byte format; gzip/unknown still rejected.
- ~~**CSP meta tag for WebView2** — recommended by the review; the frontend side is
  ready (no inline `onclick`, one delegated click handler), but the pre-paint
  theme/lang script in `index.html` must move to its own file (a static page cannot
  carry a nonce) and a strict `script-src` has to be verified against the
  Wails-injected runtime on a live `wails build`.~~ Shipped in `28484d3` and
  confirmed on a live `totalplug.exe` (stage 13) — closed.
- ~~**`domain.Config`** struct is dead code (TCPath/IniPath/Language/AutoCheck unwired);
  either wire it into persistence or delete it.~~ Deleted in `58306b6`: nothing ever read
  it, and two of its fields describe behaviour that was explicitly rejected.
- ~~**Minor `internal/config` debt** — `%COMMANDER_PATH%` expands only the first occurrence;
  no-op `existing` branch in dedup.~~ Fixed in `94f4af3`: `replaceAllFold` expands every
  case-insensitive occurrence, and the dedup branch is now a plain existence check.
- ~~**No cap on decompressed size** — found in the pre-release review: `DefaultMaxDownloadBytes`
  (256 MiB) bounds the archive on the wire, but every extraction member was copied with a plain
  `io.Copy`, so a small crafted archive could still fill the disk.~~ Shipped in `b1bb4aa` (see
  Safe updates): one `extractLimiter` shared by the three format loops caps the cumulative
  decompressed output at `DefaultMaxExtractBytes` (1 GiB), aborting with `ErrExtractTooLarge`.
- ~~**The plugin card is mouse-only** — a row receives `tabindex="-1"` only _after_ the card
  opens (`openCard` in `frontend/src/card.js`), so there is nothing to focus and therefore no way
  to open a card with the keyboard.~~ The keyboard half was already closed by the slice 4 pass
  that made the name cell a `.row-open` button: nothing under `frontend/src` writes a `tabindex`
  any more, so the entry described a mechanism that no longer exists. The names were the real
  debt and `3492c0b` paid them — the history, card and progress dialogs point `aria-labelledby`
  at the heading each already draws, the app dialog has no heading so it is named by whether it
  asks a question or reports one, and `#font-scale` and `#lang-select` reuse the labels their own
  panels show.
- ~~**Stale state across a concurrent refresh** — one known race left: a language switch rebuilds
  the DOM, so the `finally` in `runScan` enables buttons that the in-flight scan never disabled
  (harmless, but a second scan starts while the first is running).~~ Closed by the titlebar
  island: the toolbar buttons now read the `scanRunning` store instead of being DOM nodes that a
  rebuild replaces, so there is nothing left to enable behind a running scan. The two races where
  a `plugins:refreshed` landing inside an open confirm meant acting on a superseded record were
  already closed: `handleUpdate` in `0f53feb`, `handleUpdateAll` in the review pass above — each
  re-resolves the entry by stable id after the confirm and drops what is no longer offerable.
- ~~**Dead i18n keys** — `settings.about` and `about.version` are defined in both languages and
  read nowhere; the About panel renders its labels from other keys.~~ Removed in `4a9f6de`.
- **Frontend migration (Svelte 5, then Wails v3)** — Svelte migration completed in six slices,
  each merged into `main` as a working app; Wails v3 remains a separate future step. What
  decides the framework is the stage 13 policy, not taste: `style-src 'self'` without
  `'unsafe-inline'` rules out CSS-in-JS, `script-src 'self'` without `'unsafe-eval'` rules out
  Vue's runtime template compiler, and what survives is the compiled-template group — Svelte
  being the pick there. This reverses what this bullet used to say, that the framework move and
  the Wails move are one migration: the spike below ran a Svelte island inside the v2 shell with
  the Go side and the generated bindings untouched, so v2 to v3 changes only the bindings, the
  embed path and the verification pass — on a frontend that no longer has to be rewritten at the
  same time. Stage 16 was the other half of that prep: `main.js` is a composition root over one
  module per screen, so a screen can become a component without touching the others.
  Six slices are in, and the last one deleted `buildScreens()`. `#app` now splits into
  `#chrome`, `#table`, `#overlays` and `#settings` (mounted islands, all `display: contents` so
  the flex column does not move a pixel), the language is a store, and the window chrome is
  `frontend/src/Titlebar.svelte` — which is what lets a language switch leave the search box,
  its text and the keyboard focus alone. Slice 2 made `frontend/src/PluginTable.svelte` out of
  the table (`7cb9d76`): the plugin list and the in-flight update flag became stores, so
  `catalog.js` is now only stores plus the scan plus the event registration, and the rows, the
  batch bar and the placeholder are derived rather than written into a `tbody`. Two things that
  file has to keep doing: import the i18n store under another name (a component rendering `<tr>`
  cannot also hold a binding called `tr`), and keep the delegated `tbody` click ignored by the
  a11y checker — slice 4 gave the row a real button, so that click is a mouse shortcut over it
  rather than the only way in. Slice 3 put both
  dialogs in islands (`54052ff`): `dialog.js` and `progress.js` kept their exported API — callers
  still await `showConfirm()` and drive `beginProgress()` — and gave up every DOM write, while
  `Dialog.svelte` and `ProgressOverlay.svelte` own the markup. One thing stays imperative: the
  top layer has no attribute to set, so each component runs a single effect that maps the
  store's open flag onto `showModal()` and `close()`, and the three focus rules survive — safe
  button on open, opener on close, acknowledgement when the bar becomes a result. Slice 4 did
  the same to the plugin card (`218b781`): `card.js` holds the record, the two editor drafts, the
  control that opened it and the save in one `cardState` store, so `PluginCard.svelte` draws the
  info rows, the status badge and the source glyph instead of pasting strings together, and the
  last `{@html}` the card needed for itself is gone. The keyboard pass came with it — the name
  cell is a `.row-open` button now, which is where the keyboard enters a row and where focus
  returns on the dialog's `close` event, and a `<tr>` needs no `tabindex` to be handed back,
  because a row is not a control. Slice 5 took the last two screens that wrote into a DOM tree
  (`32a3643`, `347418f`). `finishProgress()` gets the result as data now — the line the user scans,
  the batch's one row per plugin, the address worth naming, a warning and the raw errors — and
  `ProgressOverlay.svelte` draws all five, so `update.js` stopped concatenating markup and the
  message gives up the `pre-wrap` its string used to need: the breaks come from the elements, only
  the disclosure body keeps the line breaks of a Go error. The live window then caught a swap slice
  3 had made (`7883e8a`) — the bar's `hidden` and the result's were written the wrong way round, so
  an update showed an empty dialog and afterwards a frozen bar with no outcome in it; the update
  itself was fine, only the two halves of the dialog were exchanged, and the two flags now read as
  the DOM writes they replaced. `history.js` kept its three round trips and handed the entries, the
  row under discussion, the failed attempt and the busy flag to one store, so `HistoryDialog.svelte`
  renders the table, the question that replaces a row and the dates, which it formats from the
  language store — a log that is already open re-formats with the rest of the screen. Every button
  there carries its own handler, so the delegated `tbody` listener left with `bindHistory()`. What
  the module still decides: `openHistory()` reads the log before the dialog opens, because a data
  folder it cannot reach belongs in the error dialog rather than in an empty window, and focus goes
  to Close on open — the browser would hand it to the first rollback otherwise — and to the confirm
  button whenever the question is on screen, including after an attempt that dropped focus by
  passing through a disabled button. Settings went last, as agreed (2026-10-16), because those forms
  already keep their values in `settings.json` and localStorage and a rebuild costs them nothing.
  Slice 6 took it (`bb9c8bc`) and deleted `buildScreens()` with it: `Settings.svelte` owns the shell
  and the two panels that only read stores — the theme cards write `theme`, the slider and the
  language select bind to `scale` and `lang` — while `TcPanel.svelte`, `BackupsPanel.svelte` and
  `AboutPanel.svelte` each draw one store their own module keeps filling from the Go side. The text
  of a field lives in that store, which is the only way a rejected edit can keep what was typed and a
  visit to another panel can leave it alone, and the reload a panel gets on entry is an effect keyed
  on `settingsSection` and `settingsOpen` — the two moments the old `openSettings()` and nav handler
  covered. `#screens` became `#settings`, `dom.js` went with the rebuild it existed for, and so did
  the `currentLang`/`setLang` mirror no screen needs any more; a language switch now re-labels the
  whole window, dialogs included, which is what "done" meant here. `HistoryDialog.svelte` picked up
  the table's rule about the `tr` import name on the way (`d120bbd`), where the compiler had warned
  on every row since slice 5. The migration is over: a new feature starts on components, at 102.71
  kB JS (gzip 36.26 — what `npm run build` answered then, stage 17 carries the current
  figure) against the 46.94 kB (gzip 13.82) the app carried before slice 1, one dependency
  in the compile path and the stage 13 policy still unrelaxed.
- **Svelte-on-v2 spike** — `svelte@5.57` with `@sveltejs/vite-plugin-svelte@6.2.4` on the existing
  `vite ^7`. The plugin's newest major asks for vite 8, which swaps the bundler for
  `rolldown 1.0.0-rc`, and that is not a layer to change right before a tag. One island component
  rendered under the page CSP with no `eval` in the bundle and no runtime-injected `<style>`,
  cost +12 kB gzip, and survived the `buildDom()` rebuild only because the spike unmounted and
  remounted it by hand — the dance slice 1 deleted together with the rebuild. Price measured on
  the real chrome: 46.94 → 76.58 kB JS, gzip 13.82 → 25.82, CSS unchanged but for the two
  `display: contents` rules. What the spike could not cover — a live WebView2 window — is covered
  now, on a built `totalplug.exe`. Formatting is gated too: `prettier --check` only sees `.svelte`
  once `prettier-plugin-svelte` is listed in `.prettierrc` by path, because the house scripts run
  prettier from the repo root, not from `frontend/`. Wails v3 itself was at `beta.29` (2026-10-13)
  with its GA milestone already a month overdue.

Nits the final review pass found — each was real, none changed behaviour a user could reach
today, and all six are now closed:

- **Two `bufio.Reader`s over `os.Stdin`** — `confirm` and `promptBlindUpdates` each wrapped
  `os.Stdin` in a fresh buffered reader, and one run can call both. Whatever the first buffered
  past its newline was gone from the second, so piped answers skipped a prompt. Both now read
  from the single reader the run builds in `main`.
- **Builtin `println` in the GUI entry** — a failed `gui.Run` was reported with the builtin,
  whose format is implementation-defined and whose stderr a Windows GUI-subsystem binary is not
  attached to, so the one error the window cannot show was also the one nobody saw. Now
  `fmt.Fprintf` on stderr.
- **`settings.json` written in place** — `os.WriteFile` truncated the file before a byte of the
  new content existed, where `internal/storage/db.go` already wrote by temp+rename. A crash
  mid-write cost the saved geometry, theme and language rather than a crash. Same shape now.
- **Bare suffix trimming in `normalizeName`** — `"plugin"` and `"ext"` were trimmed with no
  separator, so a name legitimately ending in those letters lost them (`TextExt` → `text`) and
  then matched the wrong plugin. They are cut only behind a separator, `_plugin`/`_ext` added.
- **Unused frontend assets** — `frontend/src/assets/fonts/nunito-v16-latin-regular.woff2`,
  its `OFL.txt` and `assets/images/logo-universal.png` were referenced by nothing (`style.css`
  asks for `system-ui`/`Segoe UI`). Vite bundles only what is imported, so they cost repo bytes,
  not binary bytes. Deleted.
- **The scratch sweep's mtime claim** — the comment said an update refreshed its run folder's
  timestamp "as it goes"; the download writes inside `run-<pid>/update`, which touches that
  subfolder's mtime and not the swept `run-<pid>` one. The comment was corrected first, then the
  behaviour: `config.RunScratch` now stamps the run folder on every call, and every update
  already calls it at its start, so a batch keeps the one-hour window moving instead of letting a
  second process decide the first one died. What remains exposed is a single item that runs
  longer than an hour on a batch of one — the download itself still does not move the timestamp.
