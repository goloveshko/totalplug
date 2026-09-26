package domain

import "time"

type PluginType string

const (
	TypePacker     PluginType = "WCX"
	TypeLister     PluginType = "WLX"
	TypeFileSystem PluginType = "WFX"
	TypeContent    PluginType = "WDX"
	TypeUtil       PluginType = "UTIL"
	TypeLang       PluginType = "LANG"
)

type PluginStatus string

const (
	StatusUpToDate    PluginStatus = "up_to_date"
	StatusUpdateAvail PluginStatus = "update_avail"
	StatusUncertain   PluginStatus = "uncertain"
	StatusNotFound    PluginStatus = "not_found"
	StatusError       PluginStatus = "error"
)

type VersionConfidence string

const (
	ConfidenceExact    VersionConfidence = "PE Binary"  // Exact version from PE Resource (.dll / .wlx)
	ConfidenceText     VersionConfidence = "Readme/INF" // Extracted from text docs
	ConfidenceManual   VersionConfidence = "Manual DB"  // User manually set the version in DB
	ConfidenceFallback VersionConfidence = "Unknown"    // Could not determine
)

// Override allows user to manually fix name, version, or update source for a plugin.
type Override struct {
	CustomName    string `json:"custom_name,omitempty"`    // Override display name
	CustomVersion string `json:"custom_version,omitempty"` // Override version
	CustomSource  string `json:"custom_source,omitempty"`  // e.g. "github:Alexey-T/CudaLister"
	IgnoreUpdates bool   `json:"ignore_updates,omitempty"` // Skip update check for this plugin
}

// Plugin represents an installed Total Commander plugin.
type Plugin struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Type             PluginType        `json:"type"`
	Path             string            `json:"path"`
	InstalledVersion string            `json:"installed_version"`
	LatestVersion    string            `json:"latest_version,omitempty"`
	Confidence       VersionConfidence `json:"confidence"`
	CustomSource     string            `json:"custom_source,omitempty"`
	FileHash         string            `json:"file_hash,omitempty"`
	DownloadURL      string            `json:"download_url,omitempty"`
	// DownloadURLs lists ordered candidate sources (primary first, then
	// catalog mirrors); empty means only DownloadURL is known.
	DownloadURLs []string `json:"download_urls,omitempty"`
	WebURL       string   `json:"web_url,omitempty"`
	// Source names where LatestVersion actually came from: a catalog source
	// key (e.g. "totalcmd") or the custom override ("github:Owner/Repo").
	Source      string       `json:"source,omitempty"`
	Arch        string       `json:"arch,omitempty"`
	Description string       `json:"description,omitempty"`
	Status      PluginStatus `json:"status"`
	LastChecked time.Time    `json:"last_checked,omitempty"`
}

// Config represents app-level settings.
type Config struct {
	TCPath    string `json:"tc_path"`
	IniPath   string `json:"ini_path"`
	Language  string `json:"language"`
	AutoCheck bool   `json:"auto_check"`
}

// UpdateProgress is one event emitted while an update runs, so the GUI can
// show what is happening and from where.
type UpdateProgress struct {
	// Phase is "download", "extract" or "install".
	Phase string `json:"phase"`
	// Percent is 0..100 for the download phase, or -1 when the total size is
	// unknown (indeterminate progress).
	Percent float64 `json:"percent"`
	// URL is the download source, set during the download phase.
	URL string `json:"url,omitempty"`
}

// UpdateResult reports the outcome of a GUI-initiated plugin update.
type UpdateResult struct {
	// Locked is true when the update was aborted because plugin files are
	// held by a running Total Commander; changes were rolled back.
	Locked bool `json:"locked"`
	// BackupPath is the timestamped backup directory kept after success.
	BackupPath string `json:"backup_path,omitempty"`
}

// UpdateEntry is one append-only record of a successfully applied update.
type UpdateEntry struct {
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	Version    string    `json:"version"`
	AppliedAt  time.Time `json:"applied_at"`
}

// AppInfo is what the About panel can show without a network round trip.
type AppInfo struct {
	Version     string `json:"version"`
	ReleasesURL string `json:"releases_url"`
	// DataDir is %APPDATA%\TotalPlug, named in support requests and bug reports.
	DataDir string `json:"data_dir"`
	// GoVersion is the runtime toolchain, e.g. "go1.25.0".
	GoVersion string `json:"go_version"`
}

// AppUpdate is the outcome of checking the running build against the latest
// published release.
type AppUpdate struct {
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	// Available is only true when the running version can be compared at all.
	Available bool `json:"available"`
	// Comparable is false for an unstamped development build, whose version
	// carries no digits to compare, so Latest is informative rather than an
	// update claim.
	Comparable bool      `json:"comparable"`
	ReleaseURL string    `json:"release_url,omitempty"`
	CheckedAt  time.Time `json:"checked_at,omitempty"`
}
