package domain

import "time"

type PluginType string

const (
	TypePacker     PluginType = "WCX"
	TypeLister     PluginType = "WLX"
	TypeFileSystem PluginType = "WFX"
	TypeContent    PluginType = "WDX"
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
	ConfidenceExact    VersionConfidence = "PE Binary"  // Exact version from PE Resource
	ConfidenceText     VersionConfidence = "Readme/INF" // Extracted from text docs
	ConfidenceManual   VersionConfidence = "Manual DB"  // User manually set the version
	ConfidenceFallback VersionConfidence = "Unknown"    // Could not determine
)

// Override allows user to manually fix name, version, or update source for a plugin.
type Override struct {
	CustomName    string `json:"custom_name,omitempty"`    // Override display name
	CustomVersion string `json:"custom_version,omitempty"` // Override version
	CustomSource  string `json:"custom_source,omitempty"`  // e.g. "github:Alexey-T/CudaLister" or "totalcmd:123"
	IgnoreUpdates bool   `json:"ignore_updates,omitempty"` // Skip update check for this plugin
}

type Plugin struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Type             PluginType        `json:"type"`
	Path             string            `json:"path"`
	InstalledVersion string            `json:"installed_version"`
	LatestVersion    string            `json:"latest_version"`
	Confidence       VersionConfidence `json:"confidence"`
	CustomSource     string            `json:"custom_source,omitempty"`
	FileHash         string            `json:"file_hash,omitempty"`
	DownloadURL      string            `json:"download_url,omitempty"`
	Status           PluginStatus      `json:"status"`
	LastChecked      time.Time         `json:"last_checked"`
}

type Config struct {
	TCPath    string `json:"tc_path"`
	IniPath   string `json:"ini_path"`
	Language  string `json:"language"`
	AutoCheck bool   `json:"auto_check"`
}