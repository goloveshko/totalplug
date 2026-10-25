package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
	"gopkg.in/ini.v1"

	"github.com/goloveshko/totalplug/internal/domain"
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

func FindDefaultIniPath() (string, string) {
	regIni, regDir := FindIniFromRegistry()
	if regIni != "" {
		return regIni, regDir
	}

	appData := os.Getenv("APPDATA")
	if appData != "" {
		appDataIni := filepath.Join(appData, "GHISLER", "wincmd.ini")
		if _, err := os.Stat(appDataIni); err == nil {
			// The install dir stays unknown: this ini is the roaming settings copy,
			// so its own folder is %APPDATA% and says nothing about where Total
			// Commander lives. Reporting it as the install dir used to expand
			// %COMMANDER_PATH% into %APPDATA%, which turned every plugin path into a
			// plausible-looking location that does not exist. Callers treat an empty
			// dir as "unresolved": expandTcVars leaves the placeholder alone and
			// stablePluginID falls back to the absolute path.
			return appDataIni, ""
		}
	}

	return "", ""
}

// TCLaunchers are the executables that mark a folder as a Total Commander
// root. All three names must stay in one list shared by whoever asks the
// question: install-dir detection (this package) and the updater's bounding of
// what a plugin folder may live under — a root the updater does not recognise
// makes it refuse an otherwise valid rollback. wincmd.exe is the stub the
// installers keep launching, totalcmd.exe the 32-bit binary and totalcmd64.exe
// the 64-bit one; a 64-bit install can ship only totalcmd64.exe, so accepting
// one name misses most machines.
var TCLaunchers = []string{"wincmd.exe", "totalcmd.exe", "totalcmd64.exe"}

// HasTCLauncher reports whether dir holds a Total Commander executable.
func HasTCLauncher(dir string) bool {
	if dir == "" {
		return false
	}
	for _, name := range TCLaunchers {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// IniInstallDir returns the [Configuration] InstallDir key of an ini file, which
// is where TC records the folder it was last run from. Empty when the ini has no
// such key or cannot be read.
func IniInstallDir(iniPath string) string {
	cfg, err := ini.Load(iniPath)
	if err != nil {
		return ""
	}
	// Key names in wincmd.ini are not spelled consistently across TC versions
	// (pluginbasedir and InstallDir live side by side in the same file), and
	// gopkg.in/ini matches keys case-sensitively, so look the section up by scan.
	for _, key := range cfg.Section("Configuration").Keys() {
		if strings.EqualFold(key.Name(), "InstallDir") {
			return strings.TrimSpace(os.ExpandEnv(key.Value()))
		}
	}
	return ""
}

// ResolveTCDir finds the Total Commander folder an ini file belongs to, from the
// file system alone. An empty result means nothing on disk vouched for a folder,
// and the caller decides what to guess.
//
// The order is deliberate. A folder that holds a launcher is the install it
// claims to be, so the ini's own directory and its parent - TC's portable layout
// is <root>\ini\wincmd.ini - outrank any recorded path: the ini the user pointed
// at describes the copy next to it, while InstallDir and the registry describe
// whichever machine wrote them. An ini copied out of a portable install carries
// an InstallDir that points at the original, and following it scans a Total
// Commander the user did not select.
func ResolveTCDir(iniPath string) string {
	iniDir := filepath.Dir(iniPath)
	if HasTCLauncher(iniDir) {
		return iniDir
	}
	// One level up is the whole depth of TC's documented layouts; walking further
	// would search for a launcher outside the tree the user chose.
	if parent := filepath.Dir(iniDir); HasTCLauncher(parent) {
		return parent
	}
	if recorded := IniInstallDir(iniPath); recorded != "" && HasTCLauncher(recorded) {
		return recorded
	}
	return ""
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

// genericPluginDirs are the folder names Total Commander groups plugins under
// instead of naming one: a plugin file sitting directly inside any of them has
// no folder of its own, which changes both what it should be called and whether
// anything may unpack over it.
var genericPluginDirs = map[string]bool{
	"plugins": true,
	"wcx":     true,
	"wlx":     true,
	"wfx":     true,
	"wdx":     true,
}

// IsGenericPluginDir reports whether name is one of those grouping folders,
// matched case-insensitively as every Windows path here is.
func IsGenericPluginDir(name string) bool {
	return genericPluginDirs[strings.ToLower(name)]
}

// deriveRealPluginName gets clean human-readable name from folder/file path
func deriveRealPluginName(keyName string, pluginPath string, pType domain.PluginType) string {
	// If key is numeric ("0", "1") or Packer type (WCX)
	if isNumeric(keyName) || pType == domain.TypePacker {
		dirName := filepath.Base(filepath.Dir(pluginPath))

		// Ignore generic root folder names
		if dirName != "" && !IsGenericPluginDir(dirName) {
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
			// A packer entry can carry its flags before the path ("7z=31,<path>").
			// Only a pure number before the comma is a flag: splitting at any
			// comma cut a path that itself contains one — "C:\my,dir\x.wcx"
			// turned into "dir\x.wcx" — and a real path never opens with a bare
			// digit run followed by a comma.
			if idx := strings.Index(val, ","); idx != -1 && isNumeric(val[:idx]) {
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
