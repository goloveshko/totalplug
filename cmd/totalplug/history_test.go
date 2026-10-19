package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/storage"
)

// answers feeds the prompts the way a terminal does: one buffered reader for the
// whole run, so an answer a prompt read past its own newline still reaches the
// next one.
func answers(text string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(text))
}

func writeCliFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readCliFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// cliFixture is what an update left behind: a Total Commander install whose
// plugin folder holds the newer build, the copy the update kept in the app's
// backup space, and the log line naming both.
func cliFixture(t *testing.T) (pluginDir string, db *storage.Database) {
	t.Helper()
	t.Setenv(config.DataDirEnv, t.TempDir())

	root := t.TempDir()
	writeCliFile(t, filepath.Join(root, "wincmd.exe"), "exe")
	pluginDir = filepath.Join(root, "wfx", "CudaLister")
	writeCliFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	writeCliFile(t, filepath.Join(pluginDir, "added-by-the-update.txt"), "extra")
	writeCliFile(t, filepath.Join(pluginDir, "CudaLister.ini"), "the settings the plugin saved")

	kept := filepath.Join(config.BackupSpacePath(), "wfx", "CudaLister.bak_1700000000")
	writeCliFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	db, err := storage.NewDatabase("")
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	if err := db.RecordChange(storage.ChangeFromUpdate(domain.Plugin{
		ID:               "WFX:cudalister",
		Name:             "CudaLister",
		Path:             filepath.Join(pluginDir, "cuda.wlx"),
		InstalledVersion: "1.8",
		LatestVersion:    "1.9",
	}, kept, []string{"cuda.wlx", "added-by-the-update.txt"})); err != nil {
		t.Fatalf("RecordChange: %v", err)
	}
	return pluginDir, db
}

func TestRunRollbackPutsTheKeptCopyBack(t *testing.T) {
	pluginDir, db := cliFixture(t)

	if err := runRollback(db, "cudalister", answers("y\n")); err != nil {
		t.Fatalf("runRollback: %v", err)
	}

	if got := readCliFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the older build" {
		t.Errorf("cuda.wlx = %q, want the older build", got)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "added-by-the-update.txt")); !os.IsNotExist(err) {
		t.Errorf("the file the update brought is still in the folder")
	}
	if got := readCliFile(t, filepath.Join(pluginDir, "CudaLister.ini")); got != "the settings the plugin saved" {
		t.Errorf("CudaLister.ini = %q: the rollback must leave what the plugin wrote alone", got)
	}

	hist := db.UpdateHistory()
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want the rollback logged too", len(hist))
	}
	if !hist[0].Restored || hist[0].Version != "1.8" {
		t.Errorf("newest line = restored:%v version:%q, want the rollback to 1.8", hist[0].Restored, hist[0].Version)
	}
}

// A rollback is offered by name, so the answer is asked for; declining it has to
// leave the folder exactly as it was and log nothing.
func TestRunRollbackSkipsWithoutConfirmation(t *testing.T) {
	pluginDir, db := cliFixture(t)

	if err := runRollback(db, "CudaLister", answers("n\n")); err != nil {
		t.Fatalf("runRollback: %v", err)
	}
	if got := readCliFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the newer build" {
		t.Errorf("cuda.wlx = %q, want the newer build untouched", got)
	}
	if len(db.UpdateHistory()) != 1 {
		t.Errorf("a declined rollback was logged")
	}
}

func TestRunRollbackExplainsWhyItCannot(t *testing.T) {
	_, db := cliFixture(t)

	if err := runRollback(db, "NotInLog", answers("y\n")); err == nil {
		t.Fatal("an unknown plugin name was accepted")
	} else if !strings.Contains(err.Error(), "--history") {
		t.Errorf("error should point at --history: %v", err)
	}

	if err := runRollback(db, "CudaLister", answers("y\n")); err != nil {
		t.Fatalf("first rollback: %v", err)
	}
	// The way forward from a rollback is to update again, so the second ask has
	// to say that rather than fail on the copy it no longer offers.
	if err := runRollback(db, "CudaLister", answers("y\n")); err == nil {
		t.Fatal("a rollback line was rolled back")
	} else if !strings.Contains(err.Error(), "update it again") {
		t.Errorf("error should point at updating again: %v", err)
	}
}

func TestHistoryMatchPrefersTheNewestLine(t *testing.T) {
	_, db := cliFixture(t)
	if err := runRollback(db, "CudaLister", answers("y\n")); err != nil {
		t.Fatalf("runRollback: %v", err)
	}

	entries := db.UpdateHistory()
	if len(entries) != 2 {
		t.Fatalf("history len = %d, want 2", len(entries))
	}
	match, found := historyMatch(entries, "cudaLister")
	if !found {
		t.Fatal("no line matched by name")
	}
	if !match.Restored {
		t.Errorf("matched the older line: %+v", match)
	}
	if _, found := historyMatch(entries, "WFX:cudalister"); !found {
		t.Error("no line matched by catalog id")
	}
}
