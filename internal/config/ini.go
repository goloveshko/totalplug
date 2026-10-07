package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
	"gopkg.in/ini.v1"

	"totalplug/internal/domain"
)

func isMetadataKey(keyName string) bool {
	if strings.HasPrefix(keyName, "$") {
		return true
	}
	lowerKey := strings.ToLower(keyName)
	suffixes := []string{"_detect", "_date", "_flags", "_default"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lowerKey, suffix) {
			return true
		}
	}
	return false
}

// isNumeric checks if string contains only digits (e.g. "0", "1", "2")
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func FindIniFromRegistry() (iniPath string, installDir string) {
	roots := []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE}
	subKeys := []string{
		`SOFTWARE\Ghisler\Total Commander`,
		`SOFTWARE\WOW6432Node\Ghisler\Total Commander`,
	}

	for _, root := range roots {
		for _, subKeyPath := range subKeys {
			key, err := registry.OpenKey(root, subKeyPath, registry.QUERY_VALUE)
			if err != nil {
				continue
			}

			dirVal, _, errDir := key.GetStringValue("InstallDir")
			iniVal, _, errIni := key.GetStringValue("IniFileName")
			key.Close()

			if errDir == nil && dirVal != "" {
				installDir = os.ExpandEnv(dirVal)

				if errIni == nil && iniVal != "" {
					iniPath = os.ExpandEnv(iniVal)

					if strings.HasPrefix(iniPath, ".\\") || strings.HasPrefix(iniPath, "./") {
						iniPath = filepath.Join(installDir, iniPath[2:])
					} else if !filepath.IsAbs(iniPath) {
						iniPath = filepath.Join(installDir, iniPath)
					}

					if _, err := os.Stat(iniPath); err == nil {
						return iniPath, installDir
					}
				}
			}
		}
	}

	return "", ""
}

func FindDefaultIniPath(tcDir string) (string, string) {
	regIni, regDir := FindIniFromRegistry()
	if regIni != "" {
		return regIni, regDir
	}

	if tcDir != "" {
		localIni := filepath.Join(tcDir, "wincmd.ini")
		if _, err := os.Stat(localIni); err == nil {
			return localIni, tcDir
		}
	}

	appData := os.Getenv("APPDATA")
	if appData != "" {
		appDataIni := filepath.Join(appData, "GHISLER", "wincmd.ini")
		if _, err := os.Stat(appDataIni); err == nil {
			return appDataIni, filepath.Dir(filepath.Dir(appDataIni))
		}
	}

	return "", ""
}

func expandTcVars(path string, tcDir string) string {
	if tcDir == "" {
		return os.ExpandEnv(path)
	}

	path = replaceAllFold(path, "%commander_path%", tcDir)
	path = replaceAllFold(path, "%commander_drive%", filepath.VolumeName(tcDir))

	return os.ExpandEnv(path)
}

// replaceAllFold replaces every case-insensitive occurrence of old in s with
// repl. wincmd.ini spells %COMMANDER_PATH% and %COMMANDER_DRIVE% in any case and
// a single path may reference the same variable more than once, so the
// first-match-only strings.Index the old code used left later occurrences
// unexpanded. old must be ASCII (both TC tokens are), which keeps the byte
// offsets found in the lowercased haystack valid for the original string.
func replaceAllFold(s, old, repl string) string {
	if old == "" {
		return s
	}
	lowerS := strings.ToLower(s)
	lowerOld := strings.ToLower(old)
	var b strings.Builder
	for i := 0; ; {
		idx := strings.Index(lowerS[i:], lowerOld)
		if idx == -1 {
			b.WriteString(s[i:])
			break
		}
		start := i + idx
		b.WriteString(s[i:start])
		b.WriteString(repl)
		i = start + len(lowerOld)
	}
	return b.String()
}

// deriveRealPluginName gets clean human-readable name from folder/file path
func deriveRealPluginName(keyName string, pluginPath string, pType domain.PluginType) string {
	// If key is numeric ("0", "1") or Packer type (WCX)
	if isNumeric(keyName) || pType == domain.TypePacker {
		dirName := filepath.Base(filepath.Dir(pluginPath))
		lowerDir := strings.ToLower(dirName)

		// Ignore generic root folder names
		if dirName != "" && lowerDir != "plugins" && lowerDir != "wlx" && lowerDir != "wcx" && lowerDir != "wfx" && lowerDir != "wdx" {
			return dirName
		}

		baseName := filepath.Base(pluginPath)
		ext := filepath.Ext(baseName)
		cleanName := strings.TrimSuffix(baseName, ext)
		cleanName = strings.TrimSuffix(cleanName, "64")
		cleanName = strings.TrimSuffix(cleanName, "32")
		if cleanName != "" {
			return cleanName
		}
	}
	return keyName
}

// LegacyPluginID reproduces the pre-stable-ID format ("<TYPE>_<display name>")
// so the pipeline can rekey stored overrides and records exactly once.
func LegacyPluginID(pType domain.PluginType, name string) string {
	return fmt.Sprintf("%s_%s", pType, name)
}

// stablePluginID derives the plugin identity from its path relative to the TC
// directory (lowercased, forward slashes), e.g. "WCX:plugins/7zip/total7zip.wcx".
// Moving or renaming the Total Commander install therefore keeps IDs intact;
// paths outside tcDir (or an unknown tcDir) fall back to the absolute path.
func stablePluginID(tcDir, pluginPath string, pType domain.PluginType) string {
	normalized := pluginPath
	if tcDir != "" {
		// Compare case-insensitively: Rel is purely lexical on Windows and would
		// emit "../.." hops for a differently-cased but identical prefix.
		rel, err := filepath.Rel(strings.ToLower(tcDir), strings.ToLower(pluginPath))
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			normalized = rel
		}
	}
	return fmt.Sprintf("%s:%s", pType, filepath.ToSlash(strings.ToLower(normalized)))
}

// ParsePluginsFromIni loads wincmd.ini and extracts deduplicated plugins.
func ParsePluginsFromIni(iniPath string, tcDir string) ([]domain.Plugin, error) {
	cfg, err := ini.Load(iniPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load ini file: %w", err)
	}

	uniqueMap := make(map[string]*domain.Plugin)
	var pluginOrder []string

	parseSection := func(sectionName string, pType domain.PluginType) {
		sec := cfg.Section(sectionName)
		if sec == nil {
			return
		}

		for _, key := range sec.Keys() {
			keyName := key.Name()

			if isMetadataKey(keyName) {
				continue
			}

			val := key.Value()
			if val == "" {
				continue
			}

			pluginPath := val
			if idx := strings.Index(val, ","); idx != -1 {
				pluginPath = val[idx+1:]
			}
			pluginPath = strings.TrimSpace(pluginPath)
			pluginPath = expandTcVars(pluginPath, tcDir)

			realName := deriveRealPluginName(keyName, pluginPath, pType)
			pathKey := strings.ToLower(pluginPath)

			if _, exists := uniqueMap[pathKey]; exists {
				continue
			}

			plugin := domain.Plugin{
				ID:               stablePluginID(tcDir, pluginPath, pType),
				Name:             realName,
				Type:             pType,
				Path:             pluginPath,
				InstalledVersion: "Unknown",
				Status:           domain.StatusUpToDate,
			}

			if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
				plugin.Status = domain.StatusNotFound
			}

			uniqueMap[pathKey] = &plugin
			pluginOrder = append(pluginOrder, pathKey)
		}
	}

	parseSection("PackerPlugins", domain.TypePacker)
	parseSection("ListerPlugins", domain.TypeLister)
	parseSection("FileSystemPlugins", domain.TypeFileSystem)
	parseSection("ContentPlugins", domain.TypeContent)

	var plugins []domain.Plugin
	for _, key := range pluginOrder {
		if p, ok := uniqueMap[key]; ok {
			plugins = append(plugins, *p)
		}
	}

	return plugins, nil
}
