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
- Downloads restricted to https (loopback http allowed for tests) and capped at 256 MiB.
- Mirror fallback (`7c6b18d`): the checker collects ordered download URLs per plugin from
  the catalog's `available_sources` (resolved first, then same-version mirrors, stable
  order); the updater tries each until one delivers a usable archive, so a dead
  totalcmd.net link can fall through to a wincmd.ru mirror. The catalog currently lists
  only the `totalcmd` source per entry — mirrors become effective once added to the
  totalplug-catalog repo.
- Zip-slip guarded extraction.
- Total decompressed output of one archive capped at 1 GiB (`DefaultMaxExtractBytes`,
  one `extractLimiter` shared by the ZIP/RAR/7z loops in `internal/updater`), so a small
  crafted bomb is stopped before it fills the disk — the 256 MiB wire cap alone cannot
  catch it because the archive stays tiny until it is unpacked (`b1bb4aa`).
- Files are copied into place by rename, not by truncating the target: `copyFile` writes a
  sibling temp and renames it over the destination. An in-place write that broke halfway
  left a damaged file `applyTree` had not recorded, so rollback skipped it and the backup
  holding the only good copy was then deleted as a success. The rename is also where a
  target another process holds open fails with errno 5, which is what lock detection keys on.
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

### CLI (`bin/totalplug-cli.exe`, `scripts/build.cmd` / `release.cmd`)

- Scan report with update summary table.
- `--refresh-cache / -r`, `--ini <path> / -c` (portable installs),
  `--update / -u`, `--update-all`, `--version / -v` (the stamped build version).
- Interactive confirmed updates; per-item `y/N/a` prompts for "blind" updates
  (installed version unknown); `--update-all` never touches blind updates.
- Non-zero exit code when any update fails.

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

### Engineering

- GitHub Actions CI: Windows job (`npm ci` + `npm run build` first, because the root
  `main.go` embeds `frontend/dist` and a fresh checkout has none, then `go mod verify`,
  vet, `go test -race -cover ./...`, CLI build) plus an ubuntu job that checks the
  formatting and builds the frontend on its own (`f6702e3`, `e201d1a`). Neither has run
  yet: `origin` is still a local Gitea, so the workflow is unverified until the first
  push to GitHub.
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
   full effects/theme pass. See Accepted debt below.
4. **Update progress dialog** — done (`981efbb`): «Обновить» now opens a dialog right
   after the confirm that streams `update:progress` events from the updater
   (download percent via a counting reader over the response body, then extract and
   install phases, indeterminate bar when ContentLength is unknown) and shows the
   download source URL; the final result (success + backup path, locked warning, or
   friendly error with technical details) replaces the progress in the same dialog.
   Not dismissable while running (Esc blocked, OK/✕ hidden until the end).
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
   User-side source lists were declined — mirrors belong in the catalog repo
   (`available_sources`), not in per-plugin storage.
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
      label (`.cell-type`) instead of `<strong>`; badges use monochrome glyphs
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
- **Column sorting** (at least status and name) — needs a
  sort state in `catalog.js` and stable ordering with the search filter.
- **`--json` CLI output** and a documented exit-code contract for scripting.
- **Total Commander integration docs** — README section on wiring a "check for updates"
  user-menu button to the CLI.

## Accepted debt / as needed

- **Mica/Acrylic window effect** — temporarily disabled (see stage 3). Re-enabling needs
  a proper theme pass: panels readable over any desktop backdrop, opaque titlebar,
  solid fallback — with the `window_effect` value in settings.json + the
  Get/SetWindowEffect bindings already in place.
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
- **`FileHash` verification** — field exists on records but nothing computes or checks it.
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
- **The plugin card is mouse-only** — a row receives `tabindex="-1"` only _after_ the card opens
  (`openCard` in `frontend/src/card.js`), so there is nothing to focus and therefore no way to
  open a card with the keyboard. The pass that fixes it also owes the dialogs their accessible
  names: the four `<dialog>`s carry no `aria-label`/`aria-labelledby`, and neither
  `#lang-select` nor `#font-scale` has one, while every icon button and the search box do.
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
- **Frontend migration (Svelte 5, then Wails v3)** — in progress, one slice at a time, each slice
  merged into `main` as a working app: no long-lived port branch and no double bugfixes. What
  decides the framework is the stage 13 policy, not taste: `style-src 'self'` without
  `'unsafe-inline'` rules out CSS-in-JS, `script-src 'self'` without `'unsafe-eval'` rules out
  Vue's runtime template compiler, and what survives is the compiled-template group — Svelte
  being the pick there. This reverses what this bullet used to say, that the framework move and
  the Wails move are one migration: the spike below ran a Svelte island inside the v2 shell with
  the Go side and the generated bindings untouched, so v2 to v3 changes only the bindings, the
  embed path and the verification pass — on a frontend that no longer has to be rewritten at the
  same time. Stage 16 was the other half of that prep: `main.js` is a composition root over one
  module per screen, so a screen can become a component without touching the others.
  Slice 1 is in. `#app` now splits into `#chrome` (the mounted island, `display: contents` so the
  flex column does not move a pixel) and `#screens`, the language is a store, and the window
  chrome is `frontend/src/Titlebar.svelte` — which is what lets a language switch leave the search
  box, its text and the keyboard focus alone. Next slices: the screens, one per slice, and only as
  far as each one's state has to leave the DOM.
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

Nits the final review pass found and deliberately left alone — each is real, none changes
behaviour a user can reach today:

- **Two `bufio.Reader`s over `os.Stdin`** — `confirm` (`cmd/totalplug/main.go:321`) and
  `promptBlindUpdates` (`:338`) each wrap `os.Stdin` in a fresh buffered reader, and one run can
  call both. Whatever the first buffered past its newline is gone from the second. A terminal
  hands over one line at a time so this stays invisible; piped answers would skip a prompt.
- **Builtin `println` in the GUI entry** — `main.go:16` reports a failed `gui.Run` with the
  builtin, whose format is implementation-defined and whose stderr a Windows GUI-subsystem
  binary is not attached to, so the one error the window cannot show is also the one nobody sees.
- **`settings.json` is written in place** — `internal/gui/settings.go:84` uses `os.WriteFile`
  where `internal/storage/db.go:161` already does temp+rename. A crash mid-write costs the saved
  geometry, theme and language rather than a crash: `loadSettings` logs the corrupt file and
  answers with the defaults.
- **Bare suffix trimming in `normalizeName`** — `internal/checker/checker.go` trims `"plugin"`
  and `"ext"` without a separator, so a catalog name legitimately ending in those letters loses
  them (`TextExt` → `text`) and then matches the wrong plugin.
- **Unused frontend assets** — `frontend/src/assets/fonts/nunito-v16-latin-regular.woff2`,
  its `OFL.txt` and `assets/images/logo-universal.png` are referenced by nothing (`style.css`
  asks for `system-ui`/`Segoe UI`). Vite bundles only what is imported, so they cost repo bytes,
  not binary bytes.
- **The scratch sweep's mtime claim** — `internal/config/path.go:137` says an update refreshes
  its run folder's timestamp "as it goes"; the download writes inside `run-<pid>/update`, which
  touches that subfolder's mtime and not the swept `run-<pid>` one. The hour is therefore a
  window since the run started, not since its last activity.
