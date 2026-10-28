package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestCacheDirLivesInsideDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(DataDirEnv, dir)

	got, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, cacheFolder); got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("CacheDir() must create the directory it reports: %v", err)
	}
}

func TestImportPerUserDataCopiesKnownFilesOnce(t *testing.T) {
	legacy := fakePerUserDir(t)
	if err := os.MkdirAll(filepath.Join(legacy, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	// Files the app no longer reads, written by older versions: a first portable
	// run must leave them in the per-user folder instead of filling the portable
	// one with things nobody can explain.
	junk := []string{"window_effect.txt", "github_release_cache.json", "totalcmd_catalog_cache.json"}
	for _, name := range append(append([]string{}, importableFiles...), junk...) {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	dir := filepath.Join(t.TempDir(), appFolder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	importPerUserData(dir)

	for _, name := range importableFiles {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s not imported: %v", name, err)
		}
		if string(data) != name {
			t.Errorf("%s content = %q, want %q", name, data, name)
		}
	}
	for _, name := range junk {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s must not be copied into a fresh portable folder", name)
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

func TestScratchRootHonoursEnvAndDefaultsUnderTemp(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMP", root)
	t.Setenv("TMPDIR", root)

	got, err := ScratchRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, appFolder); got != want {
		t.Fatalf("ScratchRoot() = %q, want the app's own folder inside the temp dir %q", got, want)
	}

	pinned := filepath.Join(t.TempDir(), "scratch")
	t.Setenv(ScratchEnv, pinned)
	if got, err := ScratchRoot(); err != nil || got != pinned {
		t.Fatalf("ScratchRoot() with %s set = %q (%v), want %q", ScratchEnv, got, err, pinned)
	}
}

func TestRunScratchIsOneFolderPerProcess(t *testing.T) {
	t.Setenv(ScratchEnv, filepath.Join(t.TempDir(), "scratch"))

	first, err := RunScratch()
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunScratch()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("a run must reuse one folder, got %q then %q", first, second)
	}
	// The folder carries the pid and something that makes it this process's
	// own: Windows hands a pid to a later process, and that process must not
	// find an earlier run's folder — with the packages it downloaded — under
	// its own name.
	base := filepath.Base(first)
	if !strings.HasPrefix(base, fmt.Sprintf("%s%d-", runPrefix, os.Getpid())) {
		t.Errorf("run folder = %q, want it to start with %q", base, fmt.Sprintf("%s%d-", runPrefix, os.Getpid()))
	}
}

func TestRunScratchKeepsTheRunInsideTheSweepWindow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scratch")
	t.Setenv(ScratchEnv, root)

	dir, err := RunScratch()
	if err != nil {
		t.Fatal(err)
	}
	// A run that started an hour ago is what the next sweep is meant to take;
	// starting an item has to move the folder back inside the window.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	if got := SweepScratch(time.Hour); got != 1 {
		t.Fatalf("a run that never started an item must be swept, removed %d, want 1", got)
	}

	if _, err := RunScratch(); err != nil {
		t.Fatal(err)
	}
	if got := SweepScratch(time.Hour); got != 0 {
		t.Errorf("SweepScratch() = %d over a run that just started an update, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(root, runDirName())); err != nil {
		t.Errorf("the run's folder must survive its own sweep: %v", err)
	}
}

func TestSweepScratchRemovesOnlyStaleRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scratch")
	t.Setenv(ScratchEnv, root)

	stale := filepath.Join(root, "run-1111")
	fresh := filepath.Join(root, "run-2222")
	foreign := filepath.Join(root, "keep-me")
	for _, dir := range []string{stale, fresh, foreign} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeHere := filepath.Join(dir, "package.archive")
		if err := os.WriteFile(writeHere, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if got := SweepScratch(time.Hour); got != 1 {
		t.Fatalf("SweepScratch() removed %d folder(s), want 1", got)
	}
	for _, dir := range []string{fresh, foreign} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s must survive the sweep: %v", dir, err)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("a run folder left by a dead process must be gone: %v", err)
	}
}

func TestReleaseRunScratchLeavesNothingBehind(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scratch")
	t.Setenv(ScratchEnv, root)

	dir, err := RunScratch()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.archive"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	ReleaseRunScratch()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the run's folder must be gone after release: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("an empty scratch root must not be kept: %v", err)
	}
}

func TestSweepScratchCreatesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "never-made")
	t.Setenv(ScratchEnv, root)

	if got := SweepScratch(time.Hour); got != 0 {
		t.Fatalf("SweepScratch() = %d over an absent root, want 0", got)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("a sweep must not create the folder it only reads: %v", err)
	}
}
