package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goloveshko/totalplug/internal/domain"
)

func TestExpandTcVars(t *testing.T) {
	tcDir := `C:\TotalCommander`

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "expand COMMANDER_PATH",
			input:    `%COMMANDER_PATH%\plugins\wlx\cudalister\cudalister.wlx`,
			expected: `C:\TotalCommander\plugins\wlx\cudalister\cudalister.wlx`,
		},
		{
			name:     "expand COMMANDER_DRIVE",
			input:    `%COMMANDER_DRIVE%\Tools\Plugins\plugin.wfx`,
			expected: `C:\Tools\Plugins\plugin.wfx`,
		},
		{
			name:     "no placeholders",
			input:    `D:\CustomPath\plugin.wcx`,
			expected: `D:\CustomPath\plugin.wcx`,
		},
		{
			name:     "expand every COMMANDER_PATH occurrence regardless of case",
			input:    `%COMMANDER_PATH%\plugins\%commander_path%\tool.wcx`,
			expected: `C:\TotalCommander\plugins\C:\TotalCommander\tool.wcx`,
		},
		{
			name:     "expand every COMMANDER_DRIVE occurrence",
			input:    `%COMMANDER_DRIVE%\a\%commander_drive%\b`,
			expected: `C:\a\C:\b`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTcVars(tt.input, tcDir)
			if got != tt.expected {
				t.Errorf("expandTcVars(%q, %q) = %q; want %q", tt.input, tcDir, got, tt.expected)
			}
		})
	}
}

func TestDeriveRealPluginName(t *testing.T) {
	tests := []struct {
		name       string
		keyName    string
		pluginPath string
		pType      domain.PluginType
		expected   string
	}{
		{
			name:       "numeric key with descriptive folder",
			keyName:    "0",
			pluginPath: `C:\totalcmd\plugins\wlx\CudaLister\cudalister.wlx64`,
			pType:      domain.TypeLister,
			expected:   "CudaLister",
		},
		{
			name:       "Packer extension with filename base",
			keyName:    "7z",
			pluginPath: `C:\totalcmd\plugins\wcx\Total7zip64.wcx`,
			pType:      domain.TypePacker,
			expected:   "Total7zip",
		},
		{
			name:       "named key preserved",
			keyName:    "RegistryPlugin",
			pluginPath: `C:\totalcmd\plugins\wfx\registry.wfx`,
			pType:      domain.TypeFileSystem,
			expected:   "RegistryPlugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveRealPluginName(tt.keyName, tt.pluginPath, tt.pType)
			if got != tt.expected {
				t.Errorf("deriveRealPluginName() = %q; want %q", got, tt.expected)
			}
		})
	}
}

func TestParsePluginsFromIni(t *testing.T) {
	tempDir := t.TempDir()
	iniContent := `
[PackerPlugins]
7z=31,%COMMANDER_PATH%\plugins\wcx\Total7zip\Total7zip64.wcx
zip=735,%COMMANDER_PATH%\plugins\wcx\Total7zip\Total7zip64.wcx

[ListerPlugins]
0=%COMMANDER_PATH%\plugins\wlx\CudaLister\cudalister.wlx
0_detect="FORCE"

[FileSystemPlugins]
CloudPlugin=%COMMANDER_PATH%\plugins\wfx\cloud.wfx
`
	iniPath := filepath.Join(tempDir, "wincmd.ini")
	if err := os.WriteFile(iniPath, []byte(iniContent), 0600); err != nil {
		t.Fatalf("failed to write test ini: %v", err)
	}

	tcDir := `C:\TotalCmd`
	plugins, err := ParsePluginsFromIni(iniPath, tcDir)
	if err != nil {
		t.Fatalf("ParsePluginsFromIni failed: %v", err)
	}

	// 7z and zip share the same path -> should be deduplicated to 1 packer
	// 1 lister, 1 fs -> total 3 plugins
	if len(plugins) != 3 {
		t.Fatalf("expected 3 deduplicated plugins, got %d", len(plugins))
	}

	for _, p := range plugins {
		// All non-existent on disk in unit test -> StatusNotFound
		if p.Status != domain.StatusNotFound {
			t.Errorf("expected StatusNotFound for plugin %s, got %s", p.Name, p.Status)
		}
	}
}

// The date of the plugin file itself is read where the file is named. It is
// only ever a hint — a copied file carries the time of the copy — but the GUI
// names it next to the warning about an unreadable version, and it can only
// do that if the scan brings it along.
func TestParsePluginsFromIniRecordsFileDate(t *testing.T) {
	tempDir := t.TempDir()
	pluginPath := filepath.Join(tempDir, "plugins", "wlx", "CudaLister", "cudalister.wlx")
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	written := time.Date(2026, 10, 28, 19, 5, 42, 0, time.UTC)
	if err := os.Chtimes(pluginPath, written, written); err != nil {
		t.Fatal(err)
	}

	iniPath := filepath.Join(tempDir, "wincmd.ini")
	iniContent := "[ListerPlugins]\n0=" + pluginPath + "\n"
	if err := os.WriteFile(iniPath, []byte(iniContent), 0600); err != nil {
		t.Fatal(err)
	}

	plugins, err := ParsePluginsFromIni(iniPath, tempDir)
	if err != nil {
		t.Fatalf("ParsePluginsFromIni failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].FileDate.IsZero() {
		t.Fatal("file date not recorded")
	}
	// A file system may round the stamp, so the day is what has to match.
	if diff := plugins[0].FileDate.Sub(written); diff > 2*time.Second || diff < -2*time.Second {
		t.Errorf("file date = %v; want %v", plugins[0].FileDate, written)
	}
}

// A comma in a path is the path's own: only a numeric flags prefix before the
// comma may be split off, so the entry keeps scanning as a real plugin.
func TestParsePluginsFromIniKeepsCommaInPath(t *testing.T) {
	tempDir := t.TempDir()
	iniContent := `
[PackerPlugins]
ZIP=C:\my,dir\zip.wcx
7z=31,%COMMANDER_PATH%\plugins\wcx\Total7zip\Total7zip64.wcx
`
	iniPath := filepath.Join(tempDir, "wincmd.ini")
	if err := os.WriteFile(iniPath, []byte(iniContent), 0600); err != nil {
		t.Fatalf("failed to write test ini: %v", err)
	}

	plugins, err := ParsePluginsFromIni(iniPath, `C:\TotalCmd`)
	if err != nil {
		t.Fatalf("ParsePluginsFromIni failed: %v", err)
	}

	var zipPath string
	for _, p := range plugins {
		if filepath.Base(p.Path) == "zip.wcx" {
			zipPath = p.Path
		}
	}
	if zipPath != `C:\my,dir\zip.wcx` {
		t.Errorf("path with a comma was not kept whole: %q", zipPath)
	}
}

func TestStablePluginID(t *testing.T) {
	tests := []struct {
		name       string
		tcDir      string
		pluginPath string
		pType      domain.PluginType
		expected   string
	}{
		{
			name:       "relative lowercase id",
			tcDir:      `C:\TotalCmd`,
			pluginPath: `C:\TotalCmd\plugins\wcx\Total7zip\Total7zip64.wcx`,
			pType:      domain.TypePacker,
			expected:   "WCX:plugins/wcx/total7zip/total7zip64.wcx",
		},
		{
			name:       "tc dir move keeps same id",
			tcDir:      `D:\Tools\totalcmd`,
			pluginPath: `D:\Tools\totalcmd\plugins\wcx\Total7zip\Total7zip64.wcx`,
			pType:      domain.TypePacker,
			expected:   "WCX:plugins/wcx/total7zip/total7zip64.wcx",
		},
		{
			name:       "case-insensitive prefix still relative",
			tcDir:      `C:\totalcmd`,
			pluginPath: `C:\TotalCmd\plugins\wlx\Cuda\cuda.wlx`,
			pType:      domain.TypeLister,
			expected:   "WLX:plugins/wlx/cuda/cuda.wlx",
		},
		{
			name:       "outside tcDir falls back to absolute path",
			tcDir:      `C:\TotalCmd`,
			pluginPath: `D:\Custom\plugin.wfx`,
			pType:      domain.TypeFileSystem,
			expected:   "WFX:d:/custom/plugin.wfx",
		},
		{
			name:       "other volume falls back to absolute path",
			tcDir:      ``,
			pluginPath: `C:\Elsewhere\p.wdx`,
			pType:      domain.TypeContent,
			expected:   "WDX:c:/elsewhere/p.wdx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stablePluginID(tt.tcDir, tt.pluginPath, tt.pType)
			if got != tt.expected {
				t.Errorf("stablePluginID() = %q; want %q", got, tt.expected)
			}
		})
	}
}

func TestParsePluginsFromIniIDsAreStable(t *testing.T) {
	tempDir := t.TempDir()
	iniContent := `
[PackerPlugins]
7z=31,%COMMANDER_PATH%\plugins\wcx\Total7zip\Total7zip64.wcx
`
	iniPath := filepath.Join(tempDir, "wincmd.ini")
	if err := os.WriteFile(iniPath, []byte(iniContent), 0600); err != nil {
		t.Fatal(err)
	}

	plugins, err := ParsePluginsFromIni(iniPath, `C:\TotalCmd`)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if got, want := plugins[0].ID, "WCX:plugins/wcx/total7zip/total7zip64.wcx"; got != want {
		t.Errorf("ID = %q; want %q", got, want)
	}
}

// writeTCLayout builds a folder holding a Total Commander launcher and returns it.
func writeTCLayout(t *testing.T, dir, launcher string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, launcher), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeIni(t *testing.T, path, installDir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[Configuration]\n"
	if installDir != "" {
		body += "InstallDir=" + installDir + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveTCDirPortableIniSubfolder(t *testing.T) {
	// TC's portable layout: <root>\ini\wincmd.ini, and a 64-bit install whose only
	// launcher is totalcmd64.exe.
	root := writeTCLayout(t, filepath.Join(t.TempDir(), "Total Commander"), "totalcmd64.exe")
	iniPath := writeIni(t, filepath.Join(root, "ini", "wincmd.ini"), "")

	if got := ResolveTCDir(iniPath); got != root {
		t.Errorf("ResolveTCDir = %q, want %q", got, root)
	}
}

func TestResolveTCDirPrefersTheTreeOverARecordedInstallDir(t *testing.T) {
	// An ini copied out of a portable install still names the folder it came from.
	// Believing that claim scans a Total Commander the user never picked.
	other := writeTCLayout(t, filepath.Join(t.TempDir(), "elsewhere"), "wincmd.exe")
	root := writeTCLayout(t, filepath.Join(t.TempDir(), "Total Commander"), "totalcmd64.exe")
	iniPath := writeIni(t, filepath.Join(root, "ini", "wincmd.ini"), other)

	if got := ResolveTCDir(iniPath); got != root {
		t.Errorf("ResolveTCDir = %q, want %q", got, root)
	}
}

func TestResolveTCDirUsesInstallDirForRoamingIni(t *testing.T) {
	// The installed layout keeps the ini in AppData, far from the program folder,
	// so the recorded path is the only evidence left.
	root := writeTCLayout(t, filepath.Join(t.TempDir(), "totalcmd"), "totalcmd64.exe")
	iniPath := writeIni(t, filepath.Join(t.TempDir(), "Roaming", "GHISLER", "wincmd.ini"), root)

	if got := ResolveTCDir(iniPath); got != root {
		t.Errorf("ResolveTCDir = %q, want %q", got, root)
	}
}

func TestResolveTCDirRejectsRecordedFolderWithoutALauncher(t *testing.T) {
	// A stale InstallDir that happens to exist (an emptied or moved install) is no
	// better than nothing: guessing there expands every plugin path into a folder
	// that cannot hold it.
	stale := t.TempDir()
	iniDir := filepath.Join(t.TempDir(), "copy")
	iniPath := writeIni(t, filepath.Join(iniDir, "wincmd.ini"), stale)

	if got := ResolveTCDir(iniPath); got != "" {
		t.Errorf("ResolveTCDir = %q, want unresolved", got)
	}
}

func TestIniInstallDirReadsAnyCase(t *testing.T) {
	iniPath := writeIni(t, filepath.Join(t.TempDir(), "wincmd.ini"), `C:\somewhere`)
	if got := IniInstallDir(iniPath); got != `C:\somewhere` {
		t.Errorf("IniInstallDir = %q", got)
	}

	odd := filepath.Join(t.TempDir(), "wincmd.ini")
	if err := os.WriteFile(odd, []byte("[Configuration]\ninstalldir=D:\\TotalCmd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := IniInstallDir(odd); got != `D:\TotalCmd` {
		t.Errorf("IniInstallDir(lowercased key) = %q, want %q", got, `D:\TotalCmd`)
	}
}
