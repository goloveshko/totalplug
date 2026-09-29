package config

import (
	"os"
	"path/filepath"
	"testing"
)

// fakePerUserDir points the OS config dir at a temp folder so the per-user
// fallback (and the import that reads it) never touches the real profile.
// Windows answers from APPDATA, everything else from XDG_CONFIG_HOME.
func fakePerUserDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	return filepath.Join(root, appFolder)
}

func TestDataDirHonoursEnvOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	t.Setenv(DataDirEnv, dir)

	if got := DataDir(); got != dir {
		t.Fatalf("DataDir() = %q, want %q", got, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("DataDir() must create the directory it reports: %v", err)
	}
}

func TestImportPerUserDataCopiesOnce(t *testing.T) {
	legacy := fakePerUserDir(t)
	if err := os.MkdirAll(filepath.Join(legacy, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"user_overrides.json", "window_effect.txt"} {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dir := filepath.Join(t.TempDir(), appFolder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	importPerUserData(dir)

	for _, name := range []string{"user_overrides.json", "window_effect.txt"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s not imported: %v", name, err)
		}
		if string(data) != name {
			t.Errorf("%s content = %q, want %q", name, data, name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "subdir")); !os.IsNotExist(err) {
		t.Error("subdirectories must stay in the per-user folder")
	}
	if _, err := os.Stat(filepath.Join(legacy, "user_overrides.json")); err != nil {
		t.Errorf("import must not move the originals: %v", err)
	}

	// A folder that already holds data owns it: a reset must not be undone by
	// the next launch re-importing the per-user files.
	if err := os.Remove(filepath.Join(dir, "user_overrides.json")); err != nil {
		t.Fatal(err)
	}
	importPerUserData(dir)
	if _, err := os.Stat(filepath.Join(dir, "user_overrides.json")); !os.IsNotExist(err) {
		t.Error("a non-empty portable folder must not be re-imported")
	}
}

func TestExeDirFor(t *testing.T) {
	installed := filepath.Join("D:", "totalplug", "totalplug.exe")
	if got := exeDirFor(installed); got != filepath.Dir(installed) {
		t.Fatalf("an installed binary keeps its own folder, got %q", got)
	}

	// `go run` and `go test` build into a throwaway go-build folder; data written
	// there would be deleted with the build cache.
	tempBinary := filepath.Join("D:", "go-build", "abc123", "b001", "totalplug.exe")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := exeDirFor(tempBinary); got != filepath.Join(cwd, "bin") {
		t.Fatalf("a go-build binary must resolve to the working tree's bin/, got %q", got)
	}
}

func TestResolveDataDirFallsBackWhenNoExecutableDir(t *testing.T) {
	legacy := fakePerUserDir(t)

	if got := dataDirIn(""); got != legacy {
		t.Fatalf("dataDirIn(\"\") = %q, want the per-user folder %q", got, legacy)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("the fallback folder must exist: %v", err)
	}
}
