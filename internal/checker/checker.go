package checker

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goloveshko/totalplug/internal/domain"
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
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// versionParts is a version split into the numbers it is made of and the
// letters that follow them.
type versionParts struct {
	numbers []int
	suffix  string
}

// parseVersion splits a version into its numeric core and the suffix that
// follows it. The core is the leading dotted-numeric run and the suffix is
// everything after it, so "0.6a" is the number 0.6 with the letter a and
// "1.0beta2" is the number 1.0 with the tag beta2.
//
// Parsing the core stops at the first character that is neither a digit nor a
// separator, so a tag that follows cannot corrupt the comparison: digits after
// such a character used to merge into the previous component ("2.0 x64" read as
// [2, 64] and beat "2.1").
func parseVersion(v string) versionParts {
	var numbers []int
	var current int
	var hasNum bool
	end := 0

	for i, ch := range v {
		if ch >= '0' && ch <= '9' {
			current = current*10 + int(ch-'0')
			hasNum = true
			end = i + 1
		} else if ch == '.' || ch == '-' || ch == '_' {
			if hasNum {
				numbers = append(numbers, current)
				current = 0
				hasNum = false
			}
			end = i + 1
		} else {
			break
		}
	}
	if hasNum {
		numbers = append(numbers, current)
	}
	return versionParts{numbers: numbers, suffix: v[end:]}
}

// parseVersionParts extracts the leading dotted-numeric core of a version
// string.
func parseVersionParts(v string) []int {
	return parseVersion(v).numbers
}

// The kinds of suffix a version can carry after its numeric core.
const (
	suffixNone = iota
	suffixRev
	suffixPre
	suffixOther
)

// parseSuffix reads the letters that follow the numeric core: a revision
// letter ("0.6a"), a pre-release word ("1.0beta2", "11.03 rc3"), or something
// the comparison does not understand ("1.95c_patched").
func parseSuffix(suffix string) (kind int, word string, number int) {
	s := strings.ToLower(strings.TrimSpace(suffix))

	end := 0
	for end < len(s) && isVersionLetter(s[end]) {
		end++
	}
	word = s[:end]
	if word == "" {
		return suffixNone, "", 0
	}

	// A number may follow the word, separated by a space, a dot or nothing.
	rest := strings.TrimLeft(s[end:], " .-_")
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits > 0 {
		number, _ = strconv.Atoi(rest[:digits])
	}

	switch word {
	case "alpha", "beta", "rc", "pre":
		return suffixPre, word, number
	}
	if len(word) == 1 {
		return suffixRev, word, number
	}
	return suffixOther, word, number
}

func isVersionLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// preReleaseRank orders the pre-release words: alpha before beta before rc.
func preReleaseRank(word string) int {
	switch word {
	case "alpha":
		return 0
	case "beta":
		return 1
	case "rc":
		return 2
	}
	return 0
}

// compareSuffixes orders the letters that follow the numeric core.
//
// The rules are deliberately conservative: a difference the letters cannot
// vouch for reads as equal, because an update offered on a difference we do
// not understand puts another archive over a working plugin.
//
//   - two revision letters order as letters, so 0.6a is older than 0.6c and
//     the release that carries 0.6c is not hidden any more
//   - two pre-release words order as words (alpha before beta before rc) and
//     by the number that follows them
//   - anything else reads as equal: a plain version against a tagged one
//     ("1.0beta2" and "1.0"), a revision letter against a pre-release word,
//     and a build the comparison does not understand ("1.95c_patched")
func compareSuffixes(s1, s2 string) int {
	kind1, word1, num1 := parseSuffix(s1)
	kind2, word2, num2 := parseSuffix(s2)

	if kind1 != kind2 {
		return 0
	}
	switch kind1 {
	case suffixRev:
		if word1 != word2 {
			return strings.Compare(word1, word2)
		}
	case suffixPre:
		if r1, r2 := preReleaseRank(word1), preReleaseRank(word2); r1 != r2 {
			return r1 - r2
		}
	default:
		return 0
	}

	if num1 != num2 {
		if num1 < num2 {
			return -1
		}
		return 1
	}
	return 0
}

func CompareVersions(v1, v2 string) int {
	p1 := parseVersion(cleanVersionString(v1))
	p2 := parseVersion(cleanVersionString(v2))

	maxLen := len(p1.numbers)
	if len(p2.numbers) > maxLen {
		maxLen = len(p2.numbers)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(p1.numbers) {
			n1 = p1.numbers[i]
		}
		if i < len(p2.numbers) {
			n2 = p2.numbers[i]
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return compareSuffixes(p1.suffix, p2.suffix)
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
//
// A version read out of a readme is not trusted for the decision either: it
// is a number found in prose, and the plugin that wrote it may have meant
// something else by it. Such a row is reported as uncertain, which is where
// the command line offers it for a manual look instead of applying it.
func classifyUpdate(installed, latest string, confidence domain.VersionConfidence) domain.PluginStatus {
	if !isResolvableVersion(installed) || !isResolvableVersion(latest) {
		return domain.StatusUncertain
	}
	if CompareVersions(installed, latest) < 0 {
		switch confidence {
		case domain.ConfidenceFallback, domain.ConfidenceText:
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
		s = strings.TrimPrefix(s, p)
	}

	// "plugin" and "ext" are cut only behind a separator: a name legitimately ending
	// in those letters is the plugin's own word, and cutting them made one entry
	// match a different plugin (TextExt answered as text). The width tags keep the
	// form they had.
	suffixes := []string{"64", "32", "x64", "x86", "_64", "_32", " plugin", "_plugin", " ext", "_ext"}
	for _, sfx := range suffixes {
		s = strings.TrimSuffix(s, sfx)
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
			// Nothing was compared against anything, so the "up to date" the
			// ini parser starts every row with is a claim the catalog cannot
			// support. Report the rows as uncertain instead: a green check
			// here hides every update the user is missing.
			for i := range plugins {
				if plugins[i].Status != domain.StatusNotFound {
					plugins[i].Status = domain.StatusUncertain
				}
			}
			return plugins, false, err
		}
	}

	for i := range plugins {
		p := &plugins[i]
		if p.Status == domain.StatusNotFound {
			continue
		}

		p.LastChecked = time.Now()

		// 1. Check custom override source (e.g. "github:Alexey-T/CudaLister").
		// Network pass fetches the latest release and remembers it; the
		// cache-only first paint replays that remembered result.
		if strings.HasPrefix(p.CustomSource, "github:") {
			ghRepo := strings.TrimPrefix(p.CustomSource, "github:")
			var version, downloadURL string
			var publishedAt time.Time
			found := false
			if !c.cacheOnly {
				if ghRelease, err := FetchLatestGitHubRelease(ctx, ghRepo); err == nil && ghRelease != nil && ghRelease.TagName != "" {
					version = ghRelease.TagName
					publishedAt = ghRelease.PublishedAt
					downloadURL = pickGitHubAsset(ghRelease.Assets)
					found = true
					saveGitHubRelease(ghRepo, version, downloadURL, publishedAt)
				}
			}
			if !found {
				// Either the offline paint or a live lookup that failed (rate
				// limit, timeout, a release that was deleted). Falling back to
				// the remembered release keeps the row honest: the alternative
				// is "N/A (Not in catalog)", which throws away what we know and
				// drops the plugin out of the update queue.
				if cached := loadGitHubRelease(ghRepo); cached != nil {
					version, downloadURL, publishedAt, found = cached.TagName, cached.DownloadURL, cached.PublishedAt, true
				}
			}
			if found {
				p.LatestVersion = version
				p.PublishedAt = publishedAt
				if downloadURL != "" {
					p.DownloadURL = downloadURL
				}
				p.WebURL = fmt.Sprintf("https://github.com/%s", ghRepo)
				p.Source = p.CustomSource

				p.Status = classifyUpdate(p.InstalledVersion, version, p.Confidence)
				continue
			}

			// The override names this repo as the only update source, and neither
			// a live lookup nor the remembered release answered. Falling through
			// to the community catalog would overwrite Source and DownloadURL
			// behind the user's back while CustomSource still says github:, so
			// the row stays unresolved and says why instead.
			p.LatestVersion = "N/A (GitHub unreachable)"
			p.WebURL = fmt.Sprintf("https://github.com/%s", ghRepo)
			p.Status = domain.StatusUncertain
			continue
		}

		// 2. Match against community catalog
		matchedEntry := matchCatalogEntry(*p, catalog)

		if matchedEntry != nil && matchedEntry.Resolved != nil {
			p.LatestVersion = matchedEntry.Resolved.Version
			p.PublishedAt = matchedEntry.Resolved.PublishedAt
			p.DownloadURL = matchedEntry.Resolved.DownloadURL
			p.DownloadURLs = downloadCandidates(*matchedEntry)
			p.WebURL = matchedEntry.Resolved.WebURL
			p.Source = resolvedSourceName(*matchedEntry)
			p.Arch = matchedEntry.Arch
			p.Description = matchedEntry.Description

			p.Status = classifyUpdate(p.InstalledVersion, matchedEntry.Resolved.Version, p.Confidence)
		} else {
			p.LatestVersion = "N/A (Not in catalog)"
			// An unmatched (or matched-but-unresolved) plugin was never compared
			// against a newer release, so the ini default of "up to date" is a
			// claim we cannot support: report it uncertain so the row shows "?"
			// instead of a green check that hides a catalog miss.
			p.Status = domain.StatusUncertain
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

// pickGitHubAsset chooses which release asset a github: override downloads: the
// first one named like an archive the updater can unpack. Without the filter the
// first asset was taken blindly, and a repo that attaches a checksums or notes
// file ahead of the archive delivered a file the updater then refused as an
// unsupported format. The fallback keeps repos with unusually named archives
// working exactly as before.
func pickGitHubAsset(assets []GitHubAsset) string {
	if len(assets) == 0 {
		return ""
	}
	for _, a := range assets {
		switch strings.ToLower(filepath.Ext(a.Name)) {
		case ".zip", ".rar", ".7z":
			return a.BrowserDownloadURL
		}
	}
	return assets[0].BrowserDownloadURL
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
