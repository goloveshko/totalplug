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
	StatusUpToDate    PluginStatus = "up_to_date" // Verified up to date
	StatusUpdateAvail PluginStatus = "update_avail"
	StatusUncertain   PluginStatus = "uncertain" // Version low confidence (Date/Text), needs check
	StatusNotFound    PluginStatus = "not_found" // File missing on disk
	StatusError       PluginStatus = "error"
)

// VersionConfidence indicates the reliability of the detected installed version.
type VersionConfidence string

const (
	ConfidenceExact    VersionConfidence = "PE Binary"  // 100% Exact version from PE Resource
	ConfidenceText     VersionConfidence = "Readme/INF" // Extracted from text documentation
	ConfidenceFallback VersionConfidence = "File Date"  // Fallback to File Modification Date
	ConfidenceRecorded VersionConfidence = "Local DB"   // Saved from previous TotalPlug update
)

type Plugin struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Type             PluginType        `json:"type"`
	Path             string            `json:"path"`
	InstalledVersion string            `json:"installed_version"`
	LatestVersion    string            `json:"latest_version"`
	Confidence       VersionConfidence `json:"confidence"`
	FileHash         string            `json:"file_hash"`
	DownloadURL      string            `json:"download_url"`
	Status           PluginStatus      `json:"status"`
	LastChecked      time.Time         `json:"last_checked"`
}

type Config struct {
	TCPath    string `json:"tc_path"`
	IniPath   string `json:"ini_path"`
	Language  string `json:"language"`
	AutoCheck bool   `json:"auto_check"`
}
