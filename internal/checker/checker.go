package checker

import (
	"context"
	"strings"
	"time"

	"totalplug/internal/domain"
)

type Checker struct {
	forceRefresh bool
}

func NewChecker(forceRefresh bool) *Checker {
	return &Checker{
		forceRefresh: forceRefresh,
	}
}

var knownGitHubRepos = map[string]string{
	"cudalister": "Alexey-T/CudaLister",
	"mdview":     "rweijnen/MDView",
	"total7zip":  "vdbo/Total7zip",
}

var knownAliases = map[string]string{
	"virtdisk":    "virtual disks",
	"sftpplug":    "secure ftp",
	"adbplugin":   "adb",
	"cloudplugin": "cloud",
	"davplug":     "webdav",
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

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	prefixes := []string{"tc_", "tc ", "wfx_", "wlx_", "wcx_", "wdx_"}
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

func matchesPlugin(localName string, item CatalogItem) bool {
	normLocal := normalizeName(localName)
	normTitle := normalizeName(item.Title)
	normID := normalizeName(item.ID)

	if normLocal == normTitle || normLocal == normID {
		return true
	}

	if alias, exists := knownAliases[normID]; exists {
		if normalizeName(alias) == normLocal {
			return true
		}
	}
	if alias, exists := knownAliases[normLocal]; exists {
		if normalizeName(alias) == normID || normalizeName(alias) == normTitle {
			return true
		}
	}

	return false
}

func (c *Checker) CheckUpdates(ctx context.Context, plugins []domain.Plugin) ([]domain.Plugin, bool, error) {
	catalog, fromCache, err := FetchTotalCmdCatalog(ctx, c.forceRefresh)
	if err != nil {
		return plugins, false, err
	}

	for i := range plugins {
		p := &plugins[i]
		if p.Status == domain.StatusNotFound {
			continue
		}

		p.LastChecked = time.Now()
		normLocalName := normalizeName(p.Name)

		// 1. Check custom source or known GitHub repo
		ghRepo := ""
		if strings.HasPrefix(p.CustomSource, "github:") {
			ghRepo = strings.TrimPrefix(p.CustomSource, "github:")
		} else if repo, isGH := knownGitHubRepos[normLocalName]; isGH {
			ghRepo = repo
		}

		if ghRepo != "" {
			if ghRelease, err := FetchLatestGitHubRelease(ctx, ghRepo); err == nil && ghRelease != nil {
				p.LatestVersion = ghRelease.TagName
				if len(ghRelease.Assets) > 0 {
					p.DownloadURL = ghRelease.Assets[0].BrowserDownloadURL
				}

				cmp := CompareVersions(p.InstalledVersion, ghRelease.TagName)
				if cmp < 0 {
					p.Status = domain.StatusUpdateAvail
				} else {
					p.Status = domain.StatusUpToDate
				}
				continue
			}
		}

		// 2. Fallback to totalcmd.net
		var matchedItem CatalogItem
		var found bool

		for _, catItem := range catalog {
			if matchesPlugin(p.Name, catItem) {
				matchedItem = catItem
				found = true
				break
			}
		}

		if found {
			p.LatestVersion = matchedItem.Version
			p.DownloadURL = matchedItem.DownloadURL

			cmp := CompareVersions(p.InstalledVersion, matchedItem.Version)

			if cmp < 0 {
				if p.Confidence == domain.ConfidenceFallback {
					p.Status = domain.StatusUncertain
				} else {
					p.Status = domain.StatusUpdateAvail
				}
			} else {
				p.Status = domain.StatusUpToDate
			}
		} else {
			p.LatestVersion = "N/A (Not in catalog)"
		}
	}

	return plugins, fromCache, nil
}