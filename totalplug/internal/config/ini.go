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

// isMetadataKey checks if an INI key represents TC metadata rather than a plugin path.
func isMetadataKey(keyName string) bool {
	// Skip checksum keys
	if strings.HasPrefix(keyName, "$") {
		return true
	}

	// Skip metadata suffix keys (_detect, _date, _flags, _default)
	lowerKey := strings.ToLower(keyName)
	suffixes := []string{"_detect", "_date", "_flags", "_default"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lowerKey, suffix) {
			return true
		}
	}

	return false
}

// FindIniFromRegistry searches Windows Registry for Total Commander configuration paths.
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

// FindDefaultIniPath coordinates registry search and standard fallback locations.
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

// expandTcVars replaces Total Commander specific environment variables like %COMMANDER_PATH%.
func expandTcVars(path string, tcDir string) string {
	if tcDir == "" {
		return os.ExpandEnv(path)
	}

	lowerPath := strings.ToLower(path)
	if idx := strings.Index(lowerPath, "%commander_path%"); idx != -1 {
		path = path[:idx] + tcDir + path[idx+len("%commander_path%"):]
	}

	if idx := strings.Index(strings.ToLower(path), "%commander_drive%"); idx != -1 {
		drive := filepath.VolumeName(tcDir)
		path = path[:idx] + drive + path[idx+len("%commander_drive%"):]
	}

	return os.ExpandEnv(path)
}

// ParsePluginsFromIni loads wincmd.ini and extracts all registered plugins.
func ParsePluginsFromIni(iniPath string, tcDir string) ([]domain.Plugin, error) {
	cfg, err := ini.Load(iniPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load ini file: %w", err)
	}

	var plugins []domain.Plugin

	parseSection := func(sectionName string, pType domain.PluginType) {
		sec := cfg.Section(sectionName)
		if sec == nil {
			return
		}

		for _, key := range sec.Keys() {
			keyName := key.Name()

			// Skip TC metadata keys (_detect, _date, _flags, $checksum$)
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

			plugin := domain.Plugin{
				ID:               fmt.Sprintf("%s_%s", pType, keyName),
				Name:             keyName,
				Type:             pType,
				Path:             pluginPath,
				InstalledVersion: "Unknown",
				Status:           domain.StatusUpToDate,
			}

			if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
				plugin.Status = domain.StatusNotFound
			}

			plugins = append(plugins, plugin)
		}
	}

	parseSection("PackerPlugins", domain.TypePacker)
	parseSection("ListerPlugins", domain.TypeLister)
	parseSection("FileSystemPlugins", domain.TypeFileSystem)
	parseSection("ContentPlugins", domain.TypeContent)

	return plugins, nil
}
