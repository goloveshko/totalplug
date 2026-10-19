// File: internal/checker/checker_test.go
package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int // -1 (v1 < v2), 0 (v1 == v2), 1 (v1 > v2)
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.0", "1.2.5", -1},
		{"2.0.1", "1.9.9", 1},
		{"v1.8.6.1", "1.8.6.1", 0},
		{"1.8.6.0", "1.8.6.1", -1},
		{"1.2 (build 200)", "1.2", 0},
		{"2.0", "2.0.0.0", 0},
		{"0.9.9", "1.0.0", -1},
		{"10.0.1", "2.0.5", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			got := CompareVersions(tt.v1, tt.v2)
			if got != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CudaLister", "cudalister"},
		{"wlx_cudalister_64", "cudalister"},
		{"tc_total7zip plugin", "total7zip"},
		{"TC mdview ext", "mdview"},
		{"Plugins", "plugin"},
		// A name that ends in the letters is the plugin's own word, not a tag.
		{"TextExt", "textext"},
		{"ListerPlugin", "listerplugin"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeName(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeName(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsResolvableVersion(t *testing.T) {
	resolvable := []string{"1.2.3", "v2.0", "1.2 (build 200)"}
	for _, v := range resolvable {
		if !isResolvableVersion(v) {
			t.Errorf("isResolvableVersion(%q) = false, want true", v)
		}
	}
	unresolvable := []string{"Unknown", "N/A", "", "n/a", "no version info"}
	for _, v := range unresolvable {
		if isResolvableVersion(v) {
			t.Errorf("isResolvableVersion(%q) = true, want false", v)
		}
	}
}

func TestClassifyUpdate(t *testing.T) {
	tests := []struct {
		name       string
		installed  string
		latest     string
		confidence domain.VersionConfidence
		want       domain.PluginStatus
	}{
		{"known older offers update", "1.8.6.1", "1.8.6.3", domain.ConfidenceExact, domain.StatusUpdateAvail},
		{"known current is up to date", "1.0.0", "1.0.0", domain.ConfidenceExact, domain.StatusUpToDate},
		{"known newer stays up to date", "2.0.0", "1.5.0", domain.ConfidenceExact, domain.StatusUpToDate},
		{"unknown installed never auto-updates", "Unknown", "1.8.6.3", domain.ConfidenceFallback, domain.StatusUncertain},
		{"NA installed never auto-updates", "N/A", "1.8.6.3", domain.ConfidenceExact, domain.StatusUncertain},
		{"fallback confidence blocks update", "1.0.0", "2.0.0", domain.ConfidenceFallback, domain.StatusUncertain},
		{"unparseable latest is uncertain", "1.0.0", "latest", domain.ConfidenceExact, domain.StatusUncertain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyUpdate(tt.installed, tt.latest, tt.confidence)
			if got != tt.want {
				t.Errorf("classifyUpdate(%q, %q, %q) = %q; want %q",
					tt.installed, tt.latest, tt.confidence, got, tt.want)
			}
		})
	}
}

func TestCheckUpdatesCacheOnly(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	ctx := context.Background()

	// Seed a disk cache with one resolvable entry.
	snapshot := CatalogSnapshot{Plugins: []CatalogEntry{{
		ID:   "demo-lister",
		Name: "Demo",
		Type: "lister",
		Match: MatchRule{
			Filenames: []string{"demo.wlx64"},
		},
		Resolved: &ResolvedSource{
			Version:     "2.0",
			DownloadURL: "https://example.invalid/demo.zip",
		},
	}}}
	if err := SaveCatalogToCache(snapshot, ""); err != nil {
		t.Fatal(err)
	}

	plugins := []domain.Plugin{
		{ID: "wlx_Demo", Name: "Demo", Path: `C:\tc\plugins\demo\demo.wlx64`,
			InstalledVersion: "1.0", Confidence: domain.ConfidenceExact, Status: domain.StatusUpToDate},
		// Unknown custom source must NOT trigger a network call in cache-only mode.
		{ID: "wlx_Ghost", Name: "Ghost", Path: `C:\tc\plugins\ghost\ghost.wlx`,
			InstalledVersion: "1.0", Confidence: domain.ConfidenceExact, Status: domain.StatusUpToDate,
			CustomSource: "github:ghost-cause/ghost-repo"},
	}

	got, fromCache, err := NewChecker(false, true).CheckUpdates(ctx, plugins)
	if err != nil {
		t.Fatalf("cache-only check must never fail on network state: %v", err)
	}
	if !fromCache {
		t.Error("cache-only check should report fromCache=true")
	}

	demo, ghost := got[0], got[1]
	if demo.LatestVersion != "2.0" || demo.Status != domain.StatusUpdateAvail || demo.DownloadURL == "" {
		t.Errorf("cached catalog entry not applied: %+v", demo)
	}
	// A github source is skipped offline and the plugin matches no catalog
	// entry, so it cannot be claimed up to date: an unresolved row is uncertain.
	if ghost.LatestVersion != "N/A (Not in catalog)" || ghost.Status != domain.StatusUncertain {
		t.Errorf("unresolved plugin must be uncertain, got: %+v", ghost)
	}
}

func TestCheckUpdatesCacheOnlyWithoutCache(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	plugins := []domain.Plugin{{ID: "wlx_X", Name: "X", Path: `C:\tc\x.wlx`,
		InstalledVersion: "1.0", Confidence: domain.ConfidenceExact, Status: domain.StatusUpToDate}}

	got, fromCache, err := NewChecker(true, true).CheckUpdates(context.Background(), plugins)
	if err != nil || !fromCache {
		t.Fatalf("missing cache with cacheOnly must be non-fatal: err=%v fromCache=%v", err, fromCache)
	}
	if got[0].LatestVersion != "N/A (Not in catalog)" {
		t.Errorf("expected unresolved plugin, got %+v", got[0])
	}
}

func TestCheckUpdatesCacheOnlyUsesGitHubCache(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	saveGitHubRelease("rg-software/wlx-edge-viewer", "1.2.6", "https://example.invalid/edgeview.zip")

	plugins := []domain.Plugin{{ID: "wlx_EdgeView", Name: "EdgeView",
		Path:             `C:\tc\plugins\edgeview\edgeview.wlx`,
		InstalledVersion: "1.2.5", Confidence: domain.ConfidenceExact,
		Status: domain.StatusUpToDate, CustomSource: "github:rg-software/wlx-edge-viewer"}}

	got, _, err := NewChecker(false, true).CheckUpdates(context.Background(), plugins)
	if err != nil {
		t.Fatal(err)
	}
	p := got[0]
	if p.LatestVersion != "1.2.6" || p.Status != domain.StatusUpdateAvail ||
		p.DownloadURL != "https://example.invalid/edgeview.zip" ||
		p.WebURL != "https://github.com/rg-software/wlx-edge-viewer" ||
		p.Source != "github:rg-software/wlx-edge-viewer" {
		t.Errorf("remembered github release not applied on cache-only paint: %+v", p)
	}
}

// A failed live lookup (the unauthenticated GitHub API gives 403 once the
// 60 req/h quota is spent) must not erase what the last successful check
// remembered.
func TestCheckUpdatesKeepsRememberedReleaseWhenLookupFails(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			attempts++
			w.WriteHeader(http.StatusForbidden) // what GitHub returns on a spent quota
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":1,"plugins":[]}`))
	}))
	defer srv.Close()

	oldBase, oldCatalog := GitHubAPIBase, CommunityCatalogURL
	GitHubAPIBase, CommunityCatalogURL = srv.URL, srv.URL+"/catalog.json"
	t.Cleanup(func() { GitHubAPIBase, CommunityCatalogURL = oldBase, oldCatalog })

	saveGitHubRelease("rg-software/wlx-edge-viewer", "1.2.6", "https://example.invalid/edgeview.zip")

	plugins := []domain.Plugin{{ID: "wlx_EdgeView", Name: "EdgeView",
		Path:             `C:\tc\plugins\edgeview\edgeview.wlx`,
		InstalledVersion: "1.2.5", Confidence: domain.ConfidenceExact,
		Status: domain.StatusUpToDate, CustomSource: "github:rg-software/wlx-edge-viewer"}}

	got, _, err := NewChecker(true, false).CheckUpdates(context.Background(), plugins)
	if err != nil {
		t.Fatal(err)
	}
	if attempts == 0 {
		t.Error("a network scan must still attempt the live lookup")
	}
	p := got[0]
	if p.LatestVersion != "1.2.6" || p.Status != domain.StatusUpdateAvail ||
		p.DownloadURL != "https://example.invalid/edgeview.zip" {
		t.Errorf("live failure erased the remembered release: %+v", p)
	}
}

func TestGitHubReleaseCacheRoundtrip(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	if loadGitHubRelease("owner/repo") != nil {
		t.Fatal("expected empty cache")
	}
	saveGitHubRelease("owner/repo", "2.0", "https://example.invalid/r.zip")
	saveGitHubRelease("owner/other", "3.0", "")
	// Overwrite keeps one entry per repo.
	saveGitHubRelease("owner/repo", "2.1", "https://example.invalid/r21.zip")

	repo := loadGitHubRelease("owner/repo")
	if repo == nil || repo.TagName != "2.1" || repo.DownloadURL != "https://example.invalid/r21.zip" {
		t.Errorf("roundtrip failed: %+v", repo)
	}
	if other := loadGitHubRelease("owner/other"); other == nil || other.TagName != "3.0" {
		t.Errorf("second repo lost: %+v", other)
	}
	if missing := loadGitHubRelease("owner/missing"); missing != nil {
		t.Errorf("unexpected entry: %+v", missing)
	}
}

func TestDownloadCandidates(t *testing.T) {
	entry := CatalogEntry{
		Resolved: &ResolvedSource{Version: "2.23", DownloadURL: "https://totalcmd.net/download.php?id=fi"},
		AvailableSources: map[string]ResolvedSource{
			// Same URL as resolved: must dedupe.
			"totalcmd": {Version: "2.23", DownloadURL: "https://totalcmd.net/download.php?id=fi"},
			// Same version, distinct URL: must be kept as a fallback.
			"wincmd_ru": {Version: "2.23", DownloadURL: "https://wincmd.ru/files/1140597/mirror/wlx_fi_2.23.zip"},
			// Different version: must not turn an update into a downgrade.
			"old_mirror": {Version: "2.20", DownloadURL: "https://mirror.test/old.zip"},
			// No version recorded: kept (URL carries the release).
			"anonymous": {DownloadURL: "https://other.test/fi.zip"},
		},
	}

	got := downloadCandidates(entry)
	want := []string{
		"https://totalcmd.net/download.php?id=fi",
		"https://other.test/fi.zip",
		"https://wincmd.ru/files/1140597/mirror/wlx_fi_2.23.zip",
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate[%d] = %q, want %q (order must be stable: primary first, then sorted by source name)", i, got[i], want[i])
		}
	}
}

func TestResolvedSourceName(t *testing.T) {
	resolved := &ResolvedSource{Version: "2.23", DownloadURL: "https://wincmd.ru/files/1140597/mirror/wlx_fi_2.23.zip"}
	entry := CatalogEntry{
		Resolved: resolved,
		AvailableSources: map[string]ResolvedSource{
			"totalcmd":  {Version: "2.23", DownloadURL: "https://totalcmd.net/download.php?id=fi"},
			"wincmd_ru": *resolved,
		},
	}
	if got := resolvedSourceName(entry); got != "wincmd_ru" {
		t.Errorf("resolvedSourceName = %q, want %q", got, "wincmd_ru")
	}

	// Version mismatch must not claim the source.
	wrongVersion := CatalogEntry{
		Resolved:         &ResolvedSource{Version: "2.23", DownloadURL: "https://totalcmd.net/download.php?id=fi"},
		AvailableSources: map[string]ResolvedSource{"totalcmd": {Version: "2.20", DownloadURL: "https://totalcmd.net/download.php?id=fi"}},
	}
	if got := resolvedSourceName(wrongVersion); got != "catalog" {
		t.Errorf("resolvedSourceName = %q, want %q when no source matches", got, "catalog")
	}
}
