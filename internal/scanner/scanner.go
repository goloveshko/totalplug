// Package scanner determines the installed version of a Total Commander plugin by
// trying, in order, the PE version resource of the plugin binary, the resources of
// DLLs shipped beside it, the version the plugin declares in its pluginst.inf, and
// a version string in its documentation files. The first three are trusted as
// exact: a version read out of a readme, or none at all, leaves the plugin
// uncertain.
package scanner

import "github.com/goloveshko/totalplug/internal/domain"

// EnrichPluginVersions sets each plugin's installed version and the confidence in
// it, trying the sources named in the package comment and stopping at the first
// answer. A plugin whose files are missing is marked "N/A" without being read.
func EnrichPluginVersions(plugins []domain.Plugin) []domain.Plugin {
	for i := range plugins {
		if plugins[i].Status == domain.StatusNotFound {
			plugins[i].InstalledVersion = "N/A"
			plugins[i].Confidence = domain.ConfidenceFallback
			continue
		}

		// Tier 1: Try PE Version Info on main file
		if ver, err := getFileVersion(plugins[i].Path); err == nil && ver != "" {
			plugins[i].InstalledVersion = ver
			plugins[i].Confidence = domain.ConfidenceExact
			continue
		}

		// Tier 2: Try PE Version Info on companion DLLs
		if dllVer := getFileVersionFromCompanionDLLs(plugins[i].Path); dllVer != "" {
			plugins[i].InstalledVersion = dllVer
			plugins[i].Confidence = domain.ConfidenceExact
			continue
		}

		// Tier 3: Try the version the plugin declares in its own pluginst.inf.
		// It is a structured field the plugin's author wrote, not a number
		// scraped out of prose, so it is trusted like a binary version.
		if infVer := versionFromPluginstInf(plugins[i].Path); infVer != "" {
			plugins[i].InstalledVersion = infVer
			plugins[i].Confidence = domain.ConfidenceINF
			continue
		}

		// Tier 4: Try Readme / INF / History files
		if txtVer := getVersionFromReadme(plugins[i].Path); txtVer != "" {
			plugins[i].InstalledVersion = txtVer
			plugins[i].Confidence = domain.ConfidenceText
			plugins[i].Status = domain.StatusUncertain
			continue
		}

		// Tier 5: Unknown
		plugins[i].InstalledVersion = "Unknown"
		plugins[i].Confidence = domain.ConfidenceFallback
		plugins[i].Status = domain.StatusUncertain
	}
	return plugins
}
