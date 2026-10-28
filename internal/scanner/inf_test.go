package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// writePluginstInf puts a pluginst.inf next to a plugin file and returns the
// plugin's path.
func writePluginstInf(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "demo.wlx")
	if err := os.WriteFile(pluginPath, []byte("not a real plugin"), 0o600); err != nil {
		t.Fatalf("writing plugin file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, pluginstInfName), []byte(content), 0o600); err != nil {
		t.Fatalf("writing pluginst.inf: %v", err)
	}
	return pluginPath
}

func TestVersionFromPluginstInf(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "the form real plugins ship",
			content: "[plugininstall]\n" +
				"description=Lister plugin for viewing ASN.1 files\n" +
				"type=wlx\n" +
				"file=ASNView.wlx\n" +
				"defaultdir=Plugins\\wlx\\ASNView\n" +
				"version=1.2\n",
			want: "1.2",
		},
		{
			name:    "four components are kept whole",
			content: "[plugininstall]\nversion=1.0.1.0\n",
			want:    "1.0.1.0",
		},
		{
			name:    "spaces around the separator",
			content: "[plugininstall]\nversion = 1.0.8\n",
			want:    "1.0.8",
		},
		{
			name:    "a beta tag keeps the numbers it starts with",
			content: "[plugininstall]\nversion=2.0b\n",
			want:    "2.0",
		},
		{
			name:    "a leading v is dropped",
			content: "[plugininstall]\nversion=v1.4\n",
			want:    "1.4",
		},
		{
			name:    "a quoted value is unwrapped",
			content: "[plugininstall]\nversion=\"3.1\"\n",
			want:    "3.1",
		},
		{
			name:    "a comment after the value is not part of it",
			content: "[plugininstall]\nversion=1.5 ; released 2024\n",
			want:    "1.5",
		},
		{
			name:    "the key is read whatever its case",
			content: "[plugininstall]\nVersion=4.2\n",
			want:    "4.2",
		},
		{
			name:    "a file without a version says nothing",
			content: "[plugininstall]\ndescription=CADView plugin v14.1.0.4\ntype=wlx\n",
			want:    "",
		},
		{
			name:    "a bare number is not a version to weigh",
			content: "[plugininstall]\nversion=2\n",
			want:    "",
		},
		{
			name:    "a value that is not a version is refused",
			content: "[plugininstall]\nversion=unknown\n",
			want:    "",
		},
		{
			name:    "a commented out version is not a version",
			content: "[plugininstall]\n;version=1.0\nversion=\n",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pluginPath := writePluginstInf(t, tt.content)
			if got := versionFromPluginstInf(pluginPath); got != tt.want {
				t.Errorf("versionFromPluginstInf() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionFromPluginstInfWithoutFile(t *testing.T) {
	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "demo.wlx")
	if err := os.WriteFile(pluginPath, []byte("not a real plugin"), 0o600); err != nil {
		t.Fatalf("writing plugin file: %v", err)
	}
	if got := versionFromPluginstInf(pluginPath); got != "" {
		t.Errorf("versionFromPluginstInf() = %q without a pluginst.inf, want %q", got, "")
	}
}

func TestScanFileForVersion(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"version=1.2", "1.2"},
		{"version = 1.0.8", "1.0.8"},
		{"Version: 1.2", "1.2"},
		{"Version 1.2", "1.2"},
		{"version=1.0.1.0", "1.0.1.0"},
		{"description=CADView plugin v14.1.0.4: show DWG files", "14.1.0.4"},
		{"description=Lister plugin for viewing ASN.1 files", ""},
		{"file=ASNView.wlx", ""},
		{"type=wlx", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sample.txt")
			if err := os.WriteFile(path, []byte(tt.line+"\n"), 0o600); err != nil {
				t.Fatalf("writing sample: %v", err)
			}
			if got := scanFileForVersion(path); got != tt.want {
				t.Errorf("scanFileForVersion(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
