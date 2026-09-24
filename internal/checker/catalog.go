package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// CommunityCatalogURL points to the minified JSON snapshot served by the
	// totalplug-catalog Pages site (custom domain; identical content to
	// catalog.resolved.json, just without pretty-printing).
	CommunityCatalogURL = "https://sergey.is-a.dev/totalplug-catalog/catalog.min.json"
	CacheTTL            = 24 * time.Hour
	HTTPTimeout         = 12 * time.Second
)

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
	Snapshot  CatalogSnapshot `json:"snapshot"`
}

func getCachePath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	dir := filepath.Join(appData, "TotalPlug")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "community_catalog_cache.json")
}

// LoadCatalogFromCache loads cached catalog from local disk.
func LoadCatalogFromCache() (*CatalogCache, error) {
	cacheFile := getCachePath()
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

// SaveCatalogToCache writes downloaded snapshot to local disk.
func SaveCatalogToCache(snapshot CatalogSnapshot) error {
	cache := CatalogCache{
		UpdatedAt: time.Now().UTC(),
		Snapshot:  snapshot,
	}

	cacheFile := getCachePath()
	file, err := os.Create(cacheFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cache)
}

// FetchCatalog retrieves the catalog from GitHub Pages CDN or returns 24h disk cache.
func FetchCatalog(ctx context.Context, forceRefresh bool) ([]CatalogEntry, bool, error) {
	// 1. Check local cache unless forceRefresh is requested
	if !forceRefresh {
		if cache, err := LoadCatalogFromCache(); err == nil {
			if time.Since(cache.UpdatedAt) < CacheTTL && len(cache.Snapshot.Plugins) > 0 {
				return cache.Snapshot.Plugins, true, nil // true = loaded from local cache
			}
		}
	}

	// 2. HTTP GET to our GitHub Pages endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CommunityCatalogURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0 (Total Commander Plugin Manager)")

	client := &http.Client{Timeout: HTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// Offline fallback: try to use expired cache if network is unavailable
		if cache, cacheErr := LoadCatalogFromCache(); cacheErr == nil && len(cache.Snapshot.Plugins) > 0 {
			return cache.Snapshot.Plugins, true, nil
		}
		return nil, false, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// 3. Decode JSON snapshot
	var snapshot CatalogSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, false, fmt.Errorf("failed to decode catalog JSON: %w", err)
	}

	// 4. Save to local cache
	if len(snapshot.Plugins) > 0 {
		_ = SaveCatalogToCache(snapshot)
	}

	return snapshot.Plugins, false, nil // false = fetched fresh from web
}