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
outside your Total Commander `plugins` directory and `%APPDATA%\TotalPlug`, and
every folder it changes is backed up next to it.

---

## ✨ Key Features

* 🔍 **Zero-Config Total Commander Detection:**  
  Automatically discovers `wincmd.ini` and installation paths from Windows Registry (`HKCU`/`HKLM` 32-bit & 64-bit) or `%APPDATA%`, resolving `%COMMANDER_PATH%` and `%COMMANDER_DRIVE%` variables.
* 📦 **Smart Plugin Parser & Deduplication:**  
  Extracts all Packer (`WCX`), Lister (`WLX`), File System (`WFX`), and Content (`WDX`) plugins, merging duplicate archive extensions into single logical plugins.
* 🧬 **4-Tier Version Extraction Engine:**
  1. **Tier 1 (PE Binary Resources):** Reads exact `VS_FIXEDFILEINFO` and `StringFileInfo` via Windows API `version.dll`.
  2. **Tier 2 (Companion DLLs):** Inspects companion libraries (e.g. `MediaInfo64.dll`) if the wrapper lacks metadata.
  3. **Tier 3 (Docs / INF Scanner):** Extracts version strings from `pluginst.inf`, `readme.txt`, and `history.txt`.
  4. **Tier 4 (Confidence Tags):** Transparently labels confidence (`[PE Binary]`, `[Readme/INF]`, `[Manual DB]`, `[Unknown]`).
* 🌐 **Powered by TotalPlug Community Catalog:**  
  Checks updates against the open [totalplug-catalog](https://github.com/goloveshko/totalplug-catalog) database with 24-hour offline-resilient disk caching.
* 🗜️ **Archive-Format Aware Updater:**  
  Plugin packages are dispatched by their magic bytes, so ZIP, RAR and 7z all
  install, while an unsupported container is refused with a clear message
  instead of a half-extracted folder.
* 💾 **User Overrides & Database:**  
  Persist custom names, versions, and update sources in `%APPDATA%\TotalPlug\user_overrides.json`.
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
* **Settings** — light/dark/system theme, English/Russian interface, font size.
* **About** — the running version, where your data lives, and a check for a newer
  release. TotalPlug never replaces its own files: it opens the release page.

Close Total Commander before applying updates — its process holds the plugin
files, so the first locked file stops the queue and the rest is skipped.

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
| `scripts\test.cmd`, `scripts\lint.cmd` | `go vet` + `go test -race -cover ./...`, and the linter |

Tagged builds are stamped with `git describe --tags --dirty` through `-ldflags
-X`, which is the version the About panel shows and the release check compares
against; an untagged tree builds as `dev` and is never told an update exists.

---

## 🏗️ Architecture & Project Structure

```text
totalplug/
├── cmd/totalplug/main.go     # CLI entry point: report, --update / --update-all prompts
├── internal/
│   ├── buildinfo/            # Version stamped at build time, release location
│   ├── checker/              # Catalog client + 24h cache, version compare, GitHub releases
│   ├── config/               # Windows registry & wincmd.ini resolution
│   ├── domain/               # Shared models (Plugin, Status, Confidence, AppInfo)
│   ├── pipeline/             # Scan orchestration: detect → read → compare
│   ├── scanner/              # Plugin discovery, PE / INF version extraction
│   ├── storage/              # installed_plugins.json, user overrides, update history
│   └── updater/              # Download, archive extract, backup, lock-aware install
├── frontend/                 # Wails GUI: plain HTML/CSS/JS built by Vite (no framework)
├── app.go, main.go           # Wails bridge — the Go methods the frontend may call
├── window_effect.go          # Window backdrop, resolved from window_effect.txt at startup
├── scripts/                  # build.ps1 plus the .cmd wrappers listed above
└── wails.json
```

---

## 🤝 Community Catalog

TotalPlug relies on the open community registry [totalplug-catalog](https://github.com/goloveshko/totalplug-catalog). Anyone can add or update plugins by submitting a simple Pull Request!

---

## ⚖️ License

Distributed under the [MIT License](LICENSE).