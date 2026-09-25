// File: internal/checker/checker_test.go
package checker

import (
	"context"
	"testing"

	"totalplug/internal/domain"
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
	t.Setenv("APPDATA", t.TempDir())
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
	if err := SaveCatalogToCache(snapshot); err != nil {
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
	if ghost.LatestVersion != "N/A (Not in catalog)" || ghost.Status != domain.StatusUpToDate {
		t.Errorf("github custom source should be skipped offline, got: %+v", ghost)
	}
}

func TestCheckUpdatesCacheOnlyWithoutCache(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

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
