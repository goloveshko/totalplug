# TotalPlug Community Catalog (`totalplug-catalog`)
> Open-source, community-driven database for Total Commander plugins.

## 🎯 Goal & Overview
`totalplug-catalog` is an open JSON database and registry for Total Commander plugins (`WCX`, `WLX`, `WFX`, `WDX`).
It solves the problem of missing or outdated plugin metadata on traditional platforms by leveraging **GitHub Releases API** and **Community Contributions (Pull Requests)**.

The catalog is used by the **TotalPlug** desktop/CLI application as a primary or fallback update source.

---

## 🏗️ Architectural Concept

1. **Host & CDN (0$ Server Cost):**
   - Maintained as a public GitHub repository.
   - Distributed globally via jsDelivr CDN / Raw GitHub:
     `https://cdn.jsdelivr.net/gh/<username>/totalplug-catalog@main/plugins.json`

2. **Multi-Source Support (`source_type`):**
   - **`github_release`**: Queries GitHub API (`https://api.github.com/repos/{owner}/{repo}/releases/latest`) to dynamically fetch exact version tags and `.zip` asset URLs.
   - **`direct_url`**: Points to direct zip/rar download link with specified static or dynamic version.
   - **`totalcmd_net`**: Maps plugin to totalcmd.net ID.

3. **Community Moderation Workflow:**
   - Users/Authors submit new plugins by opening a **Pull Request** to `plugins.json`.
   - GitHub Actions CI validates the JSON against `schema.json`.
   - Maintainer merges the PR -> Changes instantly go live worldwide.

---

## 📁 Repository Directory Layout

```text
totalplug-catalog/
├── README.md                 # Contribution guidelines & documentation
├── plugins.json              # Master index file containing all plugins
├── schemas/
│   └── plugin.schema.json    # JSON Schema for automated PR validation
└── .github/
    └── workflows/
        └── validate.yml      # CI workflow to check JSON formatting & schema
```

---

## 📋 JSON Schema Specification (`plugins.json`)

Example entry in `plugins.json`:

```json
[
  {
    "id": "CudaLister",
    "name": "CudaLister",
    "type": "WLX",
    "source_type": "github_release",
    "github_repo": "Alexey-T/CudaLister",
    "asset_pattern": "wlx_cudalister.*\\.zip",
    "homepage": "https://github.com/Alexey-T/CudaLister",
    "description": "Syntax highlighting viewer plugin based on CudaText engine",
    "author": "Alexey-T",
    "license": "MPL-2.0"
  },
  {
    "id": "MDView",
    "name": "MDView",
    "type": "WLX",
    "source_type": "github_release",
    "github_repo": "rweijnen/MDView",
    "homepage": "https://github.com/rweijnen/MDView",
    "description": "Markdown viewer plugin for Total Commander",
    "author": "Remer Weijnen"
  },
  {
    "id": "Total7zip",
    "name": "Total7zip",
    "type": "WCX",
    "source_type": "github_release",
    "github_repo": "vdbo/Total7zip",
    "homepage": "https://github.com/vdbo/Total7zip",
    "description": "7-Zip archiver wrapper plugin for Total Commander"
  }
]
```

---

## 🚀 Key Topics for Discussion in the New Chat:

1. **Repository Setup:**
   - Creating `totalplug-catalog` repository on GitHub.
   - Initializing `plugins.json` with popular modern plugins (`CudaLister`, `MDView`, `Total7zip`, `PEViewer`, `AkelPad`, `SumatraPDF`).

2. **Automation & CI/CD:**
   - Setting up GitHub Actions (`validate.yml`) to automatically check JSON formatting on PRs.

3. **TotalPlug Integration (Go Client):**
   - Writing `internal/checker/community.go` in the main TotalPlug Go app to fetch and parse this remote `plugins.json` with 24h caching.

4. **Web UI / Moderation Page (Future):**
   - Creating a simple web page (GitHub Pages) for browsing plugins and generating PR manifests automatically.