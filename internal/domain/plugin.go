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
	// PublishedAt is when the version in LatestVersion was released, as the
	// catalog or GitHub reports it. It is zero for plugins nothing resolved, and
	// the GUI then shows no date rather than inventing one.
	PublishedAt time.Time `json:"published_at,omitempty" ts_type:"string"`
	// ts_type is read by Wails' model generator, not by encoding/json: a struct
	// field whose type it cannot resolve is emitted as `any` and reported on the
	// console ("Not found: time.Time") on every build. A timestamp crosses the
	// bridge as an RFC 3339 string, so that is what the generated model says.
	LastChecked time.Time `json:"last_checked,omitempty" ts_type:"string"`
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
	// Insecure means no usable package was secured and an HTTP candidate needs consent.
	Insecure bool `json:"insecure"`
	// BackupPath is the timestamped backup directory kept after success.
	BackupPath string `json:"backup_path,omitempty"`
}

// UpdateEntry is one append-only record of a change the updater applied: either
// an update it installed or a rollback it put back.
type UpdateEntry struct {
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	Version    string    `json:"version"`
	AppliedAt  time.Time `json:"applied_at" ts_type:"string"`
	// PluginDir is the folder that was changed, as it was at the time. The
	// stable id cannot stand in for it: it lowercases and normalises the path,
	// and a rollback has to hand the real one to the filesystem.
	PluginDir string `json:"plugin_dir,omitempty"`
	// BackupPath is the copy of what that folder held *before* this change, so
	// rolling the line back puts that state there. It is empty for entries
	// written before the copies moved into the data folder, and the GUI drops
	// it once retention has deleted the copy.
	BackupPath string `json:"backup_path,omitempty"`
	// ReplacedVersion is the version BackupPath holds — what a rollback puts
	// back, and what the next scan then compares against.
	ReplacedVersion string `json:"replaced_version,omitempty"`
	// AppliedFiles lists, relative to PluginDir, the files this change wrote into
	// the folder. A rollback may take away only what is named here: the rest of the
	// folder is the plugin's own — it writes its settings next to its DLL — or
	// belongs to whoever installed the plugin by hand. Entries written before the
	// list existed carry nothing, and rolling one of those back puts the copy back
	// without removing anything.
	AppliedFiles []string `json:"applied_files,omitempty"`
	// Restored marks a line written by a rollback instead of an update.
	Restored bool `json:"restored,omitempty"`
}

// AppInfo is what the About panel can show without a network round trip.
type AppInfo struct {
	Version     string `json:"version"`
	ReleasesURL string `json:"releases_url"`
	// DataDir is the folder this run writes to — next to the executable for a
	// portable install, %APPDATA%\totalplug otherwise. The About panel shows it
	// because support requests and bug reports need the path, not a guess.
	DataDir string `json:"data_dir"`
	// GoVersion is the runtime toolchain, e.g. "go1.25.0".
	GoVersion string `json:"go_version"`
}

// TcConfig is the Settings panel's view of which wincmd.ini a scan reads. The
// panel needs all three at once: the stored choice, what auto-detection finds
// today, and whether the file the app would actually open is readable — a
// manual path outlives its Total Commander install as soon as the folder moves.
type TcConfig struct {
	// Configured is the manual path, empty when auto-detection is in charge.
	Configured string `json:"configured"`
	// Detected is what Registry and AppData give right now; empty when TC cannot
	// be found, which is the case a manual path is the answer to.
	Detected string `json:"detected"`
	// Effective is the one the next scan opens: Configured when set.
	Effective string `json:"effective"`
	// Readable is false when Effective names nothing usable, so the panel can
	// offer to return to auto-detection instead of failing the scan silently.
	Readable bool `json:"readable"`
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
	CheckedAt  time.Time `json:"checked_at,omitempty" ts_type:"string"`
}
