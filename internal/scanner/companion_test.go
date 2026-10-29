package scanner

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPickCompanionDLL(t *testing.T) {
	tests := []struct {
		name      string
		plugin    string
		files     []string
		want      string
		wantEmpty bool
	}{
		{
			name:   "the helper shares the plugin's name",
			plugin: "TCMediaInfo.wdx",
			files:  []string{"TCMediaInfo.wdx", "MediaInfo64.dll", "readme.txt"},
			want:   "MediaInfo64.dll",
		},
		{
			name:   "the plugin and the helper are named the same",
			plugin: "mediainfo64.wdx",
			files:  []string{"mediainfo64.wdx", "MediaInfo64.dll"},
			want:   "MediaInfo64.dll",
		},
		{
			name:   "a shared runtime belongs to nothing",
			plugin: "foo.wlx",
			files:  []string{"foo.wlx", "sqlite3.dll"},
			// The folder holds a library the plugin did not ship for itself:
			// its version says nothing about the plugin.
			wantEmpty: true,
		},
		{
			name:   "the helper extends the plugin's name",
			plugin: "iso.wlx",
			files:  []string{"iso.wlx", "isowriter.dll"},
			want:   "isowriter.dll",
		},
		{
			name:   "the plugin extends the helper's name",
			plugin: "killcopywcx2.wcx",
			files:  []string{"killcopywcx2.wcx", "killcopy.dll"},
			want:   "killcopy.dll",
		},
		{
			name:   "the build digits do not matter",
			plugin: "multiarc2012.wcx",
			files:  []string{"multiarc2012.wcx", "MultiArc.dll"},
			want:   "MultiArc.dll",
		},
		{
			name:   "the closest helper wins",
			plugin: "mediainfo.wdx",
			files:  []string{"mediainfo.wdx", "MediaInfoTools.dll", "MediaInfo64.dll"},
			want:   "MediaInfo64.dll",
		},
		{
			name:   "a plugin without a helper",
			plugin: "foo.wlx",
			files:  []string{"foo.wlx"},
			// Nothing to read: the tier answers with no version at all.
			wantEmpty: true,
		},
		{
			name:   "a name too short to belong to anything",
			plugin: "io.wlx",
			files:  []string{"io.wlx", "sqlite3.dll"},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickCompanionDLL(tt.plugin, tt.files)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("pickCompanionDLL(%q, %v) = %q, want no helper", tt.plugin, tt.files, got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("pickCompanionDLL(%q, %v) = %q, want %q", tt.plugin, tt.files, got, tt.want)
			}
		})
	}
}

func TestNameStem(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"MediaInfo64.dll", "mediainfo"},
		{"tcmediainfo.wdx", "tcmediainfo"},
		{"multiarc2012.wcx", "multiarc"},
		{"killcopywcx2.wcx", "killcopywcx"},
		{"sqlite3.dll", "sqlite"},
		{"7zip.wcx", "7zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nameStem(tt.name); got != tt.want {
				t.Errorf("nameStem(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// copySystemDLL puts a real Windows library into a folder under a name of our
// choosing. It carries a version resource, so the scanner can read one.
func copySystemDLL(t *testing.T, dir, name string) {
	t.Helper()

	source := filepath.Join(os.Getenv("SystemRoot"), "System32", "version.dll")
	in, err := os.Open(source)
	if err != nil {
		t.Skipf("no system library to copy: %v", err)
	}
	defer in.Close()

	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copying %s: %v", name, err)
	}
}

func TestGetFileVersionFromCompanionDLLs(t *testing.T) {
	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "TCMediaInfo.wdx")
	if err := os.WriteFile(pluginPath, []byte("not a real plugin"), 0o600); err != nil {
		t.Fatalf("writing the plugin: %v", err)
	}

	// A shared runtime next to the plugin says nothing about the plugin.
	copySystemDLL(t, dir, "sqlite3.dll")
	if ver := getFileVersionFromCompanionDLLs(pluginPath); ver != "" {
		t.Errorf("a library that is not the plugin's gave the version %q", ver)
	}

	// The plugin's own helper does.
	copySystemDLL(t, dir, "MediaInfo64.dll")
	if ver := getFileVersionFromCompanionDLLs(pluginPath); ver == "" {
		t.Error("the plugin's own helper gave no version")
	}
}
