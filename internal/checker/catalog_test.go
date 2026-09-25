package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testSnapshot(version string) CatalogSnapshot {
	return CatalogSnapshot{Plugins: []CatalogEntry{{
		ID:    "demo-lister",
		Name:  "Demo",
		Type:  "WLX",
		Match: MatchRule{Filenames: []string{"demo.wlx64"}},
		Resolved: &ResolvedSource{
			Version:     version,
			DownloadURL: "https://example.invalid/demo.zip",
		},
	}}}
}

func TestFetchCatalogRevalidatesWithETag(t *testing.T) {
	// Seed an expired cache that carries an ETag.
	seeded := &CatalogCache{
		UpdatedAt: time.Now().Add(-CacheTTL - time.Hour),
		ETag:      `W/"seed-etag"`,
		Snapshot:  testSnapshot("1.0"),
	}
	if err := saveCacheAtomically(seeded); err != nil {
		t.Fatal(err)
	}

	var gotIfNoneMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"seed-etag"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	oldURL := CommunityCatalogURL
	CommunityCatalogURL = server.URL
	t.Cleanup(func() { CommunityCatalogURL = oldURL })

	plugins, fromCache, err := FetchCatalog(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !fromCache || len(plugins) != 1 || plugins[0].Resolved.Version != "1.0" {
		t.Fatalf("FetchCatalog() = (%d plugins, fromCache=%v), want cached 1.0", len(plugins), fromCache)
	}
	if gotIfNoneMatch != `W/"seed-etag"` {
		t.Errorf("If-None-Match = %q, want %q", gotIfNoneMatch, `W/"seed-etag"`)
	}

	cache, err := LoadCatalogFromCache()
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(cache.UpdatedAt) > time.Minute {
		t.Errorf("cache UpdatedAt was not refreshed after 304: %v", cache.UpdatedAt)
	}
}

func TestFetchCatalogStoresNewETag(t *testing.T) {
	seeded := &CatalogCache{
		UpdatedAt: time.Now().Add(-CacheTTL - time.Hour),
		ETag:      `W/"old"`,
		Snapshot:  testSnapshot("1.0"),
	}
	if err := saveCacheAtomically(seeded); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"new"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":1,"plugins":[{"id":"demo-lister","name":"Demo","type":"WLX","match":{"filenames":["demo.wlx64"]},"resolved":{"version":"2.0","download_url":"https://example.invalid/demo.zip"}}]}`))
	}))
	defer server.Close()

	oldURL := CommunityCatalogURL
	CommunityCatalogURL = server.URL
	t.Cleanup(func() { CommunityCatalogURL = oldURL })

	plugins, fromCache, err := FetchCatalog(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if fromCache || len(plugins) != 1 || plugins[0].Resolved.Version != "2.0" {
		t.Fatalf("FetchCatalog() = (%d plugins, fromCache=%v), want fresh 2.0", len(plugins), fromCache)
	}

	cache, err := LoadCatalogFromCache()
	if err != nil {
		t.Fatal(err)
	}
	if cache.ETag != `W/"new"` {
		t.Errorf("cached ETag = %q, want %q", cache.ETag, `W/"new"`)
	}
}
