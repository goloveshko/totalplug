# 🚀 TotalPlug

> **Modern Total Commander Plugin Manager, Version Auditor & Updater** (`WCX`, `WLX`, `WFX`, `WDX`).

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/goloveshko/totalplug)](https://goreportcard.com/report/github.com/goloveshko/totalplug)
[![Catalog Powered](https://img.shields.io/badge/Catalog-totalplug--catalog-blue)](https://github.com/goloveshko/totalplug-catalog)

**TotalPlug** is a fast, lightweight, and extensible utility written in Go for managing, inspecting, and keeping your Total Commander plugins up to date.

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
* 💾 **User Overrides & Database:**  
  Persist custom names, versions, and update sources in `%APPDATA%\TotalPlug\user_overrides.json`.
* 🖥️ **Dual Interface:**  
  High-speed command-line interface (CLI) and hybrid desktop GUI powered by **Wails v2**.

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
[+] Total Commander Dir: C:\totalcmd
[+] Found wincmd.ini:    C:\totalcmd\wincmd.ini
[+] Extracting version information...
[+] Checking updates against TotalPlug community catalog...
[+] Fetched fresh catalog from GitHub CDN!

Discovered 14 installed plugins:

--- [WLX] Lister / Viewer Plugins (WLX) (5) ---
 1. CudaLister         ✓  | Ver: 1.8.6.1    | [PE Binary]    ➔ Latest: 1.8.6.1
 2. MDView             🚀 | Ver: 1.2.0      | [PE Binary]    ➔ Latest: 1.2.5
 3. EdgeViewer         ✓  | Ver: 1.4.0      | [PE Binary]    ➔ Latest: 1.4.0

--- [WCX] Packer / Archiver Plugins (WCX) (4) ---
 4. Total7zip          ✓  | Ver: 1.3.5      | [PE Binary]    ➔ Latest: 1.3.5
 5. PAXZ Archiver      ✓  | Ver: 1.0.0      | [Readme/INF]   ➔ Latest: 1.0.0

================================================================================
SUMMARY: Total 14 | 13 Valid ✓ | 1 Updates 🚀 | 0 Missing ✗ | 1 Uncertain ⚠️
================================================================================
```

---

## 🏗️ Architecture & Project Structure

```text
totalplug/
├── cmd/
│   └── totalplug/
│       └── main.go           # CLI entry point, terminal report & update runner
├── internal/
│   ├── domain/
│   │   └── plugin.go         # Core domain models (Plugin, Status, Confidence)
│   ├── config/
│   │   └── ini.go            # Windows Registry & wincmd.ini parser
│   ├── scanner/
│   │   └── version.go        # Windows PE & Document version extractor
│   ├── checker/
│   │   ├── catalog.go        # Community catalog client & 24h cache
│   │   ├── checker.go        # Semantic version comparator & smart matcher
│   │   └── github.go         # GitHub Releases fallback client
│   └── storage/
│       └── db.go             # Local overrides & update history storage
├── frontend/                 # Wails desktop GUI (HTML/CSS/JS)
├── app.go                    # Wails application backend bridge
├── go.mod
└── wails.json
```

---

## 🤝 Community Catalog

TotalPlug relies on the open community registry [totalplug-catalog](https://github.com/goloveshko/totalplug-catalog). Anyone can add or update plugins by submitting a simple Pull Request!

---

## ⚖️ License

Distributed under the [MIT License](LICENSE).