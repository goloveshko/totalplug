package domain

import "time"

// PluginType represents Total Commander plugin category.
type PluginType string

const (
	TypePacker     PluginType = "WCX" // Packer / Archiver plugins
	TypeLister     PluginType = "WLX" // File viewer plugins (F3)
	TypeFileSystem PluginType = "WFX" // File System / Virtual plugins
	TypeContent    PluginType = "WDX" // Content / Custom column plugins
)

// PluginStatus represents the current state of a plugin.
type PluginStatus string

const (
	StatusUpToDate    PluginStatus = "up_to_date"   // Plugin is up to date
	StatusUpdateAvail PluginStatus = "update_avail" // Newer version available
	StatusNotFound    PluginStatus = "not_found"    // File does not exist on disk
	StatusError       PluginStatus = "error"        // Error checking update
)

// Plugin is the primary domain entity representing a Total Commander plugin.
type Plugin struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Type             PluginType   `json:"type"`
	Path             string       `json:"path"`
	InstalledVersion string       `json:"installed_version"`
	LatestVersion    string       `json:"latest_version"`
	DownloadURL      string       `json:"download_url"`
	Status           PluginStatus `json:"status"`
	LastChecked      time.Time    `json:"last_checked"`
}

// Config represents Total Commander application configuration paths.
type Config struct {
	TCPath    string `json:"tc_path"`
	IniPath   string `json:"ini_path"`
	Language  string `json:"language"` // e.g. "en", "ru"
	AutoCheck bool   `json:"auto_check"`
}
