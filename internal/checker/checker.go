package checker

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"totalplug/internal/domain"
)

type Checker struct {
	forceRefresh bool
	cacheOnly    bool
}

// NewChecker creates an update checker. When cacheOnly is true the checker
// never touches the network: it reads the catalog from disk ignoring its TTL
// and skips live GitHub release lookups.
func NewChecker(forceRefresh, cacheOnly bool) *Checker {
	return &Checker{
		forceRefresh: forceRefresh,
		cacheOnly:    cacheOnly,
	}
}

func cleanVersionString(v string) string {
	if idx := strings.Index(v, "("); idx != -1 {
		v = v[:idx]
	}
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	return v
}

func parseVersionParts(v string) []int {
	var parts []int
	var current int
	var hasNum bool

	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			current = current*10 + int(ch-'0')
			hasNum = true
		} else if ch == '.' || ch == '-' || ch == '_' {
			if hasNum {
				parts = append(parts, current)
				current = 0
				hasNum = false
			}
		}
	}
	if hasNum {
		parts = append(parts, current)
	}
	return parts
}

func CompareVersions(v1, v2 string) int {
	p1 := parseVersionParts(cleanVersionString(v1))
	p2 := parseVersionParts(cleanVersionString(v2))

	maxLen := len(p1)
	if len(p2) > maxLen {
		maxLen = len(p2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(p1) {
			n1 = p1[i]
		}
		if i < len(p2) {
			n2 = p2[i]
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}

// isResolvableVersion reports whether the given version string carries any
// numeric component usable for comparison. It returns false for the scanner
// sentinels ("Unknown", "N/A") as well as any value without digits.
func isResolvableVersion(v string) bool {
	s := cleanVersionString(v)
	switch s {
	case "", "N/A", "Unknown":
		return false
	}
	return len(parseVersionParts(s)) > 0
}

// classifyUpdate decides the plugin status from an installed and a latest
// version. When either side cannot be resolved, or the installed version is
// only a low-confidence fallback, the result is StatusUncertain so that no
// automatic update is offered for a version we cannot actually verify.
func classifyUpdate(installed, latest string, confidence domain.VersionConfidence) domain.PluginStatus {
	if !isResolvableVersion(installed) || !isResolvableVersion(latest) {
		return domain.StatusUncertain
	}
	if CompareVersions(installed, latest) < 0 {
		if confidence == domain.ConfidenceFallback {
			return domain.StatusUncertain
		}
		return domain.StatusUpdateAvail
	}
	return domain.StatusUpToDate
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	prefixes := []string{"tc_", "tc ", "wfx_", "wlx_", "wcx_", "wdx_", "totalcmd_"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
		}
	}

	suffixes := []string{"64", "32", "x64", "x86", "_64", "_32", " plugin", "plugin", " ext", "ext"}
	for _, sfx := range suffixes {
		if strings.HasSuffix(s, sfx) {
			s = strings.TrimSuffix(s, sfx)
		}
	}

	var sb strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			sb.WriteRune(ch)
		}
	}

	result := sb.String()
	if len(result) > 3 && strings.HasSuffix(result, "s") {
		result = strings.TrimSuffix(result, "s")
	}

	return result
}

// matchesPlugin performs multi-tier matching of local installed plugin to catalog entry.
func matchesPlugin(localPlugin domain.Plugin, entry CatalogEntry) bool {
	// Tier 1: Match by exact binary filename (e.g. "cudalister.wlx64", "mdview.wlx")
	localBaseFilename := strings.ToLower(filepath.Base(localPlugin.Path))
	for _, fn := range entry.Match.Filenames {
		if strings.ToLower(fn) == localBaseFilename {
			return true
		}
	}

	// Tier 2: Match by normalized name and ID
	normLocal := normalizeName(localPlugin.Name)
	normCatalogName := normalizeName(entry.Name)
	normCatalogID := normalizeName(entry.ID)

	if normLocal == normCatalogName || normLocal == normCatalogID {
		return true
	}

	// Tier 3: Match by explicit aliases
	for _, alias := range entry.Match.Aliases {
		if normalizeName(alias) == normLocal {
			return true
		}
	}

	return false
}

// CheckUpdates resolves installed plugins against the community catalog.
func (c *Checker) CheckUpdates(ctx context.Context, plugins []domain.Plugin) ([]domain.Plugin, bool, error) {
	var catalog []CatalogEntry
	fromCache := true

	if c.cacheOnly {
		// Best-effort: an absent or unreadable cache simply leaves the catalog
		// empty; the plugin list is still valid for a fast first paint.
		if cache, err := LoadCatalogFromCache(); err == nil {
			catalog = cache.Snapshot.Plugins
		}
	} else {
		var err error
		catalog, fromCache, err = FetchCatalog(ctx, c.forceRefresh)
		if err != nil {
			return plugins, false, err
		}
	}

	for i := range plugins {
		p := &plugins[i]
		if p.Status == domain.StatusNotFound {
			continue
		}

		p.LastChecked = time.Now()

		// 1. Check custom override source (e.g. "github:Alexey-T/CudaLister")
		if strings.HasPrefix(p.CustomSource, "github:") && !c.cacheOnly {
			ghRepo := strings.TrimPrefix(p.CustomSource, "github:")
			if ghRelease, err := FetchLatestGitHubRelease(ctx, ghRepo); err == nil && ghRelease != nil {
				p.LatestVersion = ghRelease.TagName
				if len(ghRelease.Assets) > 0 {
					p.DownloadURL = ghRelease.Assets[0].BrowserDownloadURL
				}
				p.WebURL = fmt.Sprintf("https://github.com/%s", ghRepo)
				p.Source = p.CustomSource

				p.Status = classifyUpdate(p.InstalledVersion, ghRelease.TagName, p.Confidence)
				continue
			}
		}

		// 2. Match against community catalog
		var matchedEntry *CatalogEntry
		for _, entry := range catalog {
			if matchesPlugin(*p, entry) {
				matchedEntry = &entry
				break
			}
		}

		if matchedEntry != nil && matchedEntry.Resolved != nil {
			p.LatestVersion = matchedEntry.Resolved.Version
			p.DownloadURL = matchedEntry.Resolved.DownloadURL
			p.DownloadURLs = downloadCandidates(*matchedEntry)
			p.WebURL = matchedEntry.Resolved.WebURL
			p.Source = resolvedSourceName(*matchedEntry)
			p.Arch = matchedEntry.Arch
			p.Description = matchedEntry.Description

			p.Status = classifyUpdate(p.InstalledVersion, matchedEntry.Resolved.Version, p.Confidence)
		} else {
			p.LatestVersion = "N/A (Not in catalog)"
		}
	}

	return plugins, fromCache, nil
}

// downloadCandidates lists one-shot download URLs for a catalog entry: the
// resolved source first, then mirror sources that publish the same version.
// Mirrors are ordered by source name so the fallback order is stable between
// scans; scheme safety is left to the updater.
func downloadCandidates(entry CatalogEntry) []string {
	primary := entry.Resolved.DownloadURL

	seen := make(map[string]bool, len(entry.AvailableSources)+1)
	urls := make([]string, 0, len(entry.AvailableSources)+1)
	add := func(u string) {
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		urls = append(urls, u)
	}

	add(primary)
	names := make([]string, 0, len(entry.AvailableSources))
	for name := range entry.AvailableSources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		src := entry.AvailableSources[name]
		if src.Version != "" && src.Version != entry.Resolved.Version {
			continue
		}
		add(src.DownloadURL)
	}
	return urls
}

// resolvedSourceName picks the catalog source key the resolved entry came
// from (e.g. "totalcmd"), so the GUI can show where the version actually is.
// Keys are checked in name order for a stable answer when sources share URLs;
// "catalog" is the fallback when resolved matches no listed source.
func resolvedSourceName(entry CatalogEntry) string {
	names := make([]string, 0, len(entry.AvailableSources))
	for name := range entry.AvailableSources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		src := entry.AvailableSources[name]
		if src.DownloadURL == entry.Resolved.DownloadURL &&
			(src.Version == "" || src.Version == entry.Resolved.Version) {
			return name
		}
	}
	return "catalog"
}
