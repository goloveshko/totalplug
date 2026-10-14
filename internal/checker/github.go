package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/goloveshko/totalplug/internal/buildinfo"
)

// GitHubRelease represents official response from GitHub Releases API
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// GitHubAPIBase is the API root; tests point it at a local server, the same way
// catalog.go exposes CommunityCatalogURL.
var GitHubAPIBase = "https://api.github.com"

// FetchLatestGitHubRelease retrieves latest release info from a GitHub repository (e.g., "Alexey-T/CudaLister")
func FetchLatestGitHubRelease(ctx context.Context, repo string) (*GitHubRelease, error) {
	repo = strings.TrimPrefix(repo, "https://github.com/")
	repo = strings.TrimSuffix(repo, "/")

	url := fmt.Sprintf("%s/repos/%s/releases/latest", GitHubAPIBase, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TotalPlug/"+buildinfo.Version+" (Total Commander Plugin Manager)")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	// Clean tag_name (e.g. "v1.8.6.0" -> "1.8.6.0")
	release.TagName = strings.TrimPrefix(release.TagName, "v")

	return &release, nil
}

// githubCachedRelease is a persisted last-known-good release lookup, so a
// cache-only scan can show the override's version immediately instead of
// "N/A (Not in catalog)" until the background network pass lands.
type githubCachedRelease struct {
	TagName     string    `json:"tag_name"`
	DownloadURL string    `json:"download_url,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

const githubCacheFile = "github_releases.json"

func loadGitHubReleaseCache() map[string]githubCachedRelease {
	path, err := cacheFilePath(githubCacheFile)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache map[string]githubCachedRelease
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return cache
}

// loadGitHubRelease returns the remembered release for a repo, or nil when
// nothing was cached yet (first run after adding the override).
func loadGitHubRelease(repo string) *githubCachedRelease {
	if entry, ok := loadGitHubReleaseCache()[repo]; ok {
		return &entry
	}
	return nil
}

// saveGitHubRelease remembers a fresh network result. Best-effort: cache
// problems must never fail a scan, and a corrupt file is rewritten from
// scratch on the next successful fetch.
func saveGitHubRelease(repo, tagName, downloadURL string) {
	cache := loadGitHubReleaseCache()
	if cache == nil {
		cache = map[string]githubCachedRelease{}
	}
	cache[repo] = githubCachedRelease{
		TagName:     tagName,
		DownloadURL: downloadURL,
		CheckedAt:   time.Now().UTC(),
	}

	path, err := cacheFilePath(githubCacheFile)
	if err != nil {
		return
	}
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(cache)
}
