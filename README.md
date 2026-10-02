# 🚀 TotalPlug

> **Modern Total Commander Plugin Manager, Version Auditor & Updater** (`WCX`, `WLX`, `WFX`, `WDX`).

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/goloveshko/totalplug)](https://goreportcard.com/report/github.com/goloveshko/totalplug)
[![Catalog Powered](https://img.shields.io/badge/Catalog-totalplug--catalog-blue)](https://github.com/goloveshko/totalplug-catalog)

**TotalPlug** is a fast, lightweight, and extensible utility written in Go for managing, inspecting, and keeping your Total Commander plugins up to date.

---

## 📥 Download

Prebuilt Windows x64 binaries are on the [Releases page](https://github.com/goloveshko/totalplug/releases):

| File | What it is |
| --- | --- |
| `totalplug.exe` | Desktop GUI (Wails window, no installer — run it from any folder) |
| `totalplug-cli.exe` | Command-line scanner and updater |

Both need Windows 10 or 11; the GUI renders through the WebView2 runtime that
already ships with Windows 11 and with current Windows 10 updates.

The binaries are not code-signed, so SmartScreen shows "Windows protected your
PC" on the first run — pick **More info → Run anyway**. TotalPlug writes nothing
outside your Total Commander `plugins` directory and its own data folder (see
**Where Your Files Live**), and every folder it changes is backed up next to it.

---

## 📁 Where Your Files Live

TotalPlug keeps everything it writes in one folder, picked at every start:

* **Portable** — a `totalplug` folder next to the `.exe`, whenever that folder
  accepts writes. Copy the app's folder to another machine or a USB stick and the
  whole setup travels with it, WebView2 profile (theme, language, font size)
  included. On the first portable run the database, the overrides, the update
  history and the settings are copied out of `%APPDATA%\totalplug` — only the
  files this version reads, so old caches and retired files stay behind. The
  originals are left alone, so nothing is lost by trying.
* **Per-user** — `%APPDATA%\totalplug`, when the executable sits somewhere
  read-only: Program Files, a write-protected share, a mounted image.

Inside that folder the plugin database, the overrides, the update history and
the window settings sit at the top level, the downloaded catalog and GitHub
release snapshots go to `cache\`, and the WebView2 profile to `webview2\`.
Deleting `cache\` only costs the next download; deleting `webview2\` resets the
theme, language and font size.

The About panel prints the folder in use. `TOTALPLUG_DATA_DIR` pins it to any
path you choose.

---

## ✨ Key Features

* 🔍 **Zero-Config Total Commander Detection:**  
  Automatically discovers `wincmd.ini` and installation paths from Windows Registry (`HKCU`/`HKLM` 32-bit & 64-bit) or `%APPDATA%`, resolving `%COMMANDER_PATH%` and `%COMMANDER_DRIVE%` variables. A Total Commander the registry does not know about — a portable one, usually — can be named by hand instead.
* 📦 **Smart Plugin Parser & Deduplication:**  
  Extracts all Packer (`WCX`), Lister (`WLX`), File System (`WFX`), and Content (`WDX`) plugins, merging duplicate archive extensions into single logical plugins.
* 🧬 **4-Tier Version Extraction Engine:**
  1. **Tier 1 (PE Binary Resources):** Reads exact `VS_FIXEDFILEINFO` and `StringFileInfo` via Windows API `version.dll`.
  2. **Tier 2 (Companion DLLs):** Inspects companion libraries (e.g. `MediaInfo64.dll`) if the wrapper lacks metadata.
  3. **Tier 3 (Docs / INF Scanner):** Extracts version strings from `pluginst.inf`, `readme.txt`, and `history.txt`.
  4. **Tier 4 (Confidence Tags):** Transparently labels confidence (`[PE Binary]`, `[Readme/INF]`, `[Manual DB]`, `[Unknown]`).
* 🌐 **Powered by TotalPlug Community Catalog:**  
  Checks updates against the open [totalplug-catalog](https://github.com/goloveshko/totalplug-catalog) database. The on-disk copy is revalidated by ETag on every launch — a newly published plugin shows up on the next start, an unchanged catalog costs a 304 instead of a full download — and it is what answers when the network does not.
* 🗜️ **Archive-Format Aware Updater:**  
  Plugin packages are dispatched by their magic bytes, so ZIP, RAR and 7z all
  install, while an unsupported container is refused with a clear message
  instead of a half-extracted folder.
* 💾 **User Overrides & Database:**  
  Persist custom names, versions, and update sources in `user_overrides.json`,
  inside the data folder TotalPlug picks for you.
* 🖥️ **Dual Interface:**  
  High-speed command-line interface (CLI) and hybrid desktop GUI powered by **Wails v2**.

---

## 🖥️ Desktop GUI

`totalplug.exe` shows the same audit the CLI prints as a table you can act on:

* **Update all** — a bar above the table appears whenever updates are available
  and applies the whole list in one pass, one plugin at a time. Plugins whose
  installed version could not be determined stay out of the batch, exactly like
  `--update-all` on the command line.
* **Per-plugin update** — the button in the row, for the cases a batch skips.
* **Plugin card** — click a row for the description, update source, homepage and
  install path, plus an editor for a manual version/source override when
  auto-detection got it wrong.
* **History** — the updates TotalPlug applied, with version and timestamp.
* **Settings** — light/dark/system theme, English/Russian interface, font size,
  and a Total Commander panel that names the `wincmd.ini` the next scan reads:
  auto-detected, or one you pick when the registry does not know your install.
* **Remembered window** — the size, position and maximised state you closed with
  are what the next launch opens with; only the very first run starts at the
  default 1024×768.
* **About** — the running version, where your data lives, and a check for a newer
  release. TotalPlug never replaces its own files: it opens the release page.

Close Total Commander before applying updates — its process holds the plugin
files, so the first locked file stops the queue and the rest is skipped.

---

## 📸 Screenshots

The same window in both themes, with a scanned install of 28 plugins.

### Light

| Main window | Plugin card | About |
| --- | --- | --- |
| ![The plugin audit table: type, plugin, installed version, catalog version and status badge per row, with a bar above it offering to update all six available updates at once](screenshots/light/main_window.png) | ![A plugin card opened from a row: installed and catalog versions with their confidence and source, status, homepage link, and the editor for a manual version or update-source override](screenshots/light/details.png) | ![The About panel inside Settings: the running version, a check for a newer release, and links to the plugin catalog and the releases page](screenshots/light/about.png) |

### Dark

| Main window | Plugin card | About |
| --- | --- | --- |
| ![The plugin audit table in the dark theme, same columns and the same update-all bar](screenshots/dark/main_window.png) | ![The plugin card in the dark theme, with the override editor below the details](screenshots/dark/details.png) | ![The About panel in the dark theme](screenshots/dark/about.png) |

---

## 🚀 Quick Start (CLI)

### Run from Source
```bash
go run cmd/totalplug/main.go
```

### Force Refresh Catalog Cache
```bash
go run cmd/totalplug/main.go --refresh-cache
```

### Apply Available Updates
Interactive mode — lists pending updates and asks for confirmation:
```bash
go run cmd/totalplug/main.go --update
```
Non-interactive mode — applies every available update without prompting (for scripts and Task Scheduler):
```bash
go run cmd/totalplug/main.go --update-all
```
> Close Total Commander before applying updates, otherwise locked plugin files will fail to replace.
> Updates are safe by design: the plugin folder is backed up as `<dir>.bak_<timestamp>` next to it,
> and any mid-apply failure is automatically rolled back to the previous state.
> Plugins whose installed version cannot be determined are never auto-updated: in `--update` they
> are offered separately with a per-item `[y]es / [N]o / [a]ll` prompt, and `--update-all` skips them.

### Point to a Portable Total Commander Install
```bash
go run cmd/totalplug/main.go --ini D:\TC\wincmd.ini
```

### Check Which Build You Have
```bash
go run cmd/totalplug/main.go --version
```
Prints the version stamped at build time — the same value the GUI shows on its
About panel. `go run` passes no `-ldflags`, so this command reports the unstamped
`dev` even in a tagged tree; use `scripts\build.cmd` for a numbered binary.

### Example Output

```text
================================================================================
                     TotalPlug - Total Commander Updater
================================================================================
[+] Scanning Total Commander plugins...
[+] Total Commander Dir: C:\totalcmd
[+] Found wincmd.ini:    C:\totalcmd\wincmd.ini
[+] Fetched fresh catalog from GitHub CDN!

Discovered 14 installed plugins:

--- [WCX] Packer / Archiver Plugins (WCX) (4) ---
 1. Total7zip          ✓  | Ver: 1.3.5      | [PE Binary]    ➔ Latest: 1.3.5
 2. PAXZ Archiver      ⚠️ | Ver: 1.0.0      | [Readme/INF]   ➔ Latest: 1.0.0

--- [WLX] Lister / Viewer Plugins (WLX) (5) ---
 3. CudaLister         ✓  | Ver: 1.8.6.1    | [PE Binary]    ➔ Latest: 1.8.6.1
 4. MDView             🚀 | Ver: 1.2.0      | [PE Binary]    ➔ Latest: 1.2.5

================================================================================
SUMMARY: Total 14 | 13 Valid ✓ | 1 Updates 🚀 | 0 Missing ✗ | 1 Uncertain ⚠️
================================================================================
--------------------------------------------------------------------------------
LEGEND:
  ✓  - Up to date
  🚀 - Update available
  ⚠️ - Uncertain version (check manually or force update)
  ✗  - Plugin file missing on disk

[🚀] SUCCESS: Found 1 plugin update(s) available!
```

---

## 🔨 Build from source

Requirements: Go 1.25+, Node 22+, and for the GUI the Wails v2 CLI
(`go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0`).

| Command | Result |
| --- | --- |
| `scripts\build.cmd` | `bin\totalplug-cli.exe`, development build |
| `scripts\build-gui.cmd` | `bin\totalplug.exe` (also accepts extra `wails` flags, e.g. `-devtools`) |
| `scripts\release.cmd`, `scripts\release-gui.cmd` | Same, with `-trimpath` and stripped symbols |
| `scripts\test.cmd` | `go test -v ./...`, extra arguments pass through to `go test` |
| `scripts\lint.cmd` | `go vet ./...`, plus `golangci-lint` when it is installed |

Tagged builds are stamped with `git describe --tags --dirty` through `-ldflags
-X`, which is the version the About panel shows and the release check compares
against; before the first tag exists the build scripts fall back to the version
tracked in `wails.json`, so an untagged tree compares too. Only a binary built by
hand carries no stamp (`dev`) and is never told an update exists.
Publishing a version (tag → build → release assets) is a documented procedure:
see [docs/release.md](docs/release.md).

---

## 🏗️ Architecture & Project Structure

```text
totalplug/
├── cmd/totalplug/main.go     # CLI entry point: report, --update / --update-all prompts
├── main.go                   # GUI entry point: embeds frontend/dist, hands it to gui.Run
├── internal/
│   ├── buildinfo/            # Version stamped at build time, release location
│   ├── checker/              # Catalog client + disk cache, version compare, GitHub releases
│   ├── config/               # Registry & wincmd.ini resolution, data folder layout
│   ├── domain/               # Shared models (Plugin, Status, Confidence, AppInfo)
│   ├── gui/                  # Wails bridge — the Go methods the frontend may call, window state
│   ├── pipeline/             # Scan orchestration: detect → read → compare
│   ├── scanner/              # Plugin discovery, PE / INF version extraction
│   ├── storage/              # installed_plugins.json, user overrides, update history
│   └── updater/              # Download, archive extract, backup, lock-aware install
├── frontend/                 # Wails GUI: plain HTML/CSS/JS built by Vite (no framework)
├── scripts/                  # build.ps1 plus the .cmd wrappers listed above
└── wails.json
```

---

## 🤝 Community Catalog

TotalPlug relies on the open community registry [totalplug-catalog](https://github.com/goloveshko/totalplug-catalog). Anyone can add or update plugins by submitting a simple Pull Request!

---

## ⚖️ License

Distributed under the [MIT License](LICENSE).