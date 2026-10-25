package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/goloveshko/totalplug/internal/config"
)

const (
	HTTPTimeout = 12 * time.Second
)

// CommunityCatalogURL points to the minified JSON snapshot served by the
// totalplug-catalog Pages site (custom domain; identical content to
// catalog.resolved.json, just without pretty-printing).
// Declared as var so tests can point it at a local server.
var CommunityCatalogURL = "https://sergey.is-a.dev/totalplug-catalog/catalog.min.json"

// MatchRule defines how TotalPlug matches local files to catalog items.
type MatchRule struct {
	Aliases   []string `json:"aliases,omitempty"`
	Filenames []string `json:"filenames"`
}

// SourceInfo defines where to retrieve updates.
type SourceInfo struct {
	Type         string `json:"type"`
	Repo         string `json:"repo,omitempty"`
	AssetPattern string `json:"asset_pattern,omitempty"`
	Version      string `json:"version,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`
	TotalcmdID   string `json:"totalcmd_id,omitempty"`
}

// ResolvedSource holds dynamically fetched version and download links.
type ResolvedSource struct {
	Version      string    `json:"version"`
	DownloadURL  string    `json:"download_url"`
	WebURL       string    `json:"web_url,omitempty"`
	Arch         string    `json:"arch,omitempty"`
	HasSource    bool      `json:"has_source,omitempty"`
	PublishedAt  time.Time `json:"published_at,omitempty"`
	ResolvedFrom string    `json:"resolved_from"` // "github", "direct", "totalcmd"
}

// CatalogEntry represents a single plugin from totalplug-catalog.
type CatalogEntry struct {
	ID               string                    `json:"id"`
	Name             string                    `json:"name"`
	Type             string                    `json:"type"`
	Category         string                    `json:"category,omitempty"`
	Description      string                    `json:"description,omitempty"`
	Authors          []string                  `json:"authors,omitempty"`
	Homepage         string                    `json:"homepage,omitempty"`
	License          string                    `json:"license,omitempty"`
	Arch             string                    `json:"arch,omitempty"`
	HasSource        bool                      `json:"has_source,omitempty"`
	Match            MatchRule                 `json:"match"`
	Source           SourceInfo                `json:"source"`
	Resolved         *ResolvedSource           `json:"resolved,omitempty"`
	AvailableSources map[string]ResolvedSource `json:"available_sources,omitempty"`
}

// CatalogSnapshot is the top-level structure received from CDN.
type CatalogSnapshot struct {
	Version     int            `json:"version"`
	GeneratedAt time.Time      `json:"generated_at"`
	TotalCount  int            `json:"total_count"`
	Plugins     []CatalogEntry `json:"plugins"`
}

// CatalogCache represents disk cache structure on the client.
type CatalogCache struct {
	UpdatedAt time.Time       `json:"updated_at"`
	ETag      string          `json:"etag,omitempty"`
	Snapshot  CatalogSnapshot `json:"snapshot"`
}

// catalogCacheFile is the on-disk catalog snapshot. It lives in the data
// folder's cache directory, whose files are all re-downloadable.
const catalogCacheFile = "community_catalog.json"

// cacheFilePath resolves a cache file inside the data folder's cache directory
// (next to the executable for a portable install, %APPDATA%\totalplug
// otherwise).
func cacheFilePath(name string) (string, error) {
	dir, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// LoadCatalogFromCache loads cached catalog from local disk.
func LoadCatalogFromCache() (*CatalogCache, error) {
	cacheFile, err := cacheFilePath(catalogCacheFile)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(cacheFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cache CatalogCache
	if err := json.NewDecoder(file).Decode(&cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveCatalogToCache writes downloaded snapshot to local disk along with the
// CDN ETag used for conditional revalidation.
func SaveCatalogToCache(snapshot CatalogSnapshot, etag string) error {
	return saveCacheAtomically(&CatalogCache{
		UpdatedAt: time.Now().UTC(),
		ETag:      etag,
		Snapshot:  snapshot,
	})
}

// saveCacheAtomically writes the cache to a temp file first, then renames it
// over the target, so a crash mid-write cannot corrupt the existing cache.
func saveCacheAtomically(cache *CatalogCache) error {
	cacheFile, err := cacheFilePath(catalogCacheFile)
	if err != nil {
		return err
	}
	tmpFile := cacheFile + ".tmp"

	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	// Compact JSON, mirroring catalog.min.json on the CDN.
	if err := json.NewEncoder(file).Encode(cache); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmpFile)
		return err
	}
	return os.Rename(tmpFile, cacheFile)
}

// FetchCatalog always revalidates the catalog against the CDN with
// If-None-Match, so an unchanged catalog costs a small 304 instead of the full
// ~700 KB download, and a newly published plugin shows up on the next launch
// rather than hiding behind a 24h window. The on-disk cache still answers the
// instant offline paint (checker's cache-only path) and this function's offline
// fallback; forceRefresh skips revalidation and re-downloads outright.
func FetchCatalog(ctx context.Context, forceRefresh bool) ([]CatalogEntry, bool, error) {
	cache, cacheErr := LoadCatalogFromCache()
	haveCache := cacheErr == nil && len(cache.Snapshot.Plugins) > 0

	// 1. HTTP GET to our GitHub Pages endpoint, revalidating when possible. A
	// forced refresh drops the ETag so the CDN cannot answer 304 and hand back
	// the very snapshot the user is trying to replace.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CommunityCatalogURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0 (Total Commander Plugin Manager)")
	if !forceRefresh && haveCache && cache.ETag != "" {
		req.Header.Set("If-None-Match", cache.ETag)
	}

	client := newCheckClient(HTTPTimeout)
	resp, err := client.Do(req)
	if err != nil {
		// 2. Offline fallback: serve the cache when the network is unavailable.
		if haveCache {
			return cache.Snapshot.Plugins, true, nil
		}
		return nil, false, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	// 3. Content unchanged: refresh the cache timestamp, keep the snapshot.
	if resp.StatusCode == http.StatusNotModified && haveCache {
		cache.UpdatedAt = time.Now().UTC()
		_ = saveCacheAtomically(cache)
		return cache.Snapshot.Plugins, true, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// 4. Decode JSON snapshot
	var snapshot CatalogSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, false, fmt.Errorf("failed to decode catalog JSON: %w", err)
	}

	// 5. Save to local cache with the new ETag for future revalidation.
	if len(snapshot.Plugins) > 0 {
		_ = SaveCatalogToCache(snapshot, resp.Header.Get("ETag"))
	}

	return snapshot.Plugins, false, nil // false = fetched fresh from web
}
