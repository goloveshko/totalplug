package config

import (
	"os"
	"path/filepath"
	"testing"

	"totalplug/internal/domain"
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
