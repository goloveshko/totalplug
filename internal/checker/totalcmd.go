package checker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CatalogItem represents a single plugin from totalcmd.net database.
type CatalogItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	ReleaseDate string `json:"release_date"`
	Category    string `json:"category"`
	Arch        string `json:"arch"`
	DownloadURL string `json:"download_url"`
}

// CatalogCache represents disk cache structure.
type CatalogCache struct {
	UpdatedAt time.Time     `json:"updated_at"`
	Items     []CatalogItem `json:"items"`
}

const (
	TotalCmdListURL    = "https://totalcmd.net/get_plugins_list.php"
	TotalCmdDetailsURL = "https://totalcmd.net/get_plugin_details.php"
	CacheTTL           = 24 * time.Hour
)

func getCachePath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	dir := filepath.Join(appData, "TotalPlug")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "totalcmd_catalog_cache.json")
}

// LoadCatalogFromCache loads cached totalcmd.net plugins from disk.
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

// SaveCatalogToCache writes totalcmd.net plugin items to disk cache.
func SaveCatalogToCache(items []CatalogItem) error {
	cache := CatalogCache{
		UpdatedAt: time.Now(),
		Items:     items,
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

// FetchTotalCmdCatalog fetches the catalog from totalcmd.net or returns 24h cache.
func FetchTotalCmdCatalog(ctx context.Context, forceRefresh bool) ([]CatalogItem, bool, error) {
	// 1. Check existing cache unless forceRefresh is true
	if !forceRefresh {
		if cache, err := LoadCatalogFromCache(); err == nil {
			if time.Since(cache.UpdatedAt) < CacheTTL {
				return cache.Items, true, nil // true = loaded from cache
			}
		}
	}

	// 2. Make HTTP GET request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, TotalCmdListURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0 (Total Commander Plugin Manager)")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Fallback to expired cache if network is unavailable
		if cache, cacheErr := LoadCatalogFromCache(); cacheErr == nil && len(cache.Items) > 0 {
			return cache.Items, true, nil
		}
		return nil, false, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// 3. Parse pipe-separated format: ID|Title|Version|Date|Category|Arch
	var items []CatalogItem
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		id := strings.TrimSpace(parts[0])
		item := CatalogItem{
			ID:          id,
			Title:       strings.TrimSpace(parts[1]),
			Version:     strings.TrimSpace(parts[2]),
			ReleaseDate: strings.TrimSpace(parts[3]),
			Category:    strings.TrimSpace(parts[4]),
			Arch:        strings.TrimSpace(parts[5]),
			DownloadURL: fmt.Sprintf("https://totalcmd.net/download.php?id=%s", id),
		}
		items = append(items, item)
	}

	// 4. Save to cache file
	if len(items) > 0 {
		_ = SaveCatalogToCache(items)
	}

	return items, false, nil // false = fetched live from web
}

// FetchPluginDetails retrieves description/changelog HTML for a specific plugin.
func FetchPluginDetails(ctx context.Context, pluginID string, lang string) (string, error) {
	if lang == "" {
		lang = "ru"
	}
	url := fmt.Sprintf("%s?id=%s&lang=%s", TotalCmdDetailsURL, pluginID, lang)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
