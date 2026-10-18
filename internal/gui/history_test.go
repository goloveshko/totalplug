package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/storage"
)

func writeGuiFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// A copy retention has taken away must stop being offered as something to roll
// back to, while the line that names it stays: it still records what happened.
func TestGetUpdateHistoryDropsADeletedCopy(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	app := NewApp()

	kept := filepath.Join(t.TempDir(), "CudaLister.bak_1700000000")
	writeGuiFile(t, filepath.Join(kept, "cuda.wlx"), "older")
	gone := filepath.Join(t.TempDir(), "ACDCommand.bak_1700000000")

	for _, backup := range []string{kept, gone} {
		if err := app.db.RecordChange(storage.AppliedChange{
			PluginID:   "WFX:" + filepath.Base(backup),
			PluginName: filepath.Base(backup),
			Version:    "1.9",
			BackupPath: backup,
		}); err != nil {
			t.Fatal(err)
		}
	}

	hist, err := app.GetUpdateHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	if hist[0].BackupPath != "" {
		t.Errorf("a deleted copy is still named: %q", hist[0].BackupPath)
	}
	if hist[1].BackupPath != kept {
		t.Errorf("a kept copy was dropped: %q, want %q", hist[1].BackupPath, kept)
	}
}

// The binding is what turns a history line back into a folder on disk, so it is
// also where the rollback has to be logged and the remembered version has to
// follow the folder.
func TestRollbackUpdateRestoresAndLogs(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	app := NewApp()

	root := t.TempDir()
	writeGuiFile(t, filepath.Join(root, "wincmd.exe"), "exe")
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeGuiFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")

	kept := filepath.Join(config.BackupSpacePath(), "wfx", "CudaLister.bak_1700000000")
	writeGuiFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")
	// One file the update brought and one the plugin wrote for itself: the first
	// is named by the change and goes, the second is not and stays.
	writeGuiFile(t, filepath.Join(pluginDir, "added-by-the-update.txt"), "extra")
	writeGuiFile(t, filepath.Join(pluginDir, "CudaLister.ini"), "the settings the plugin saved")

	if err := app.db.RecordChange(storage.ChangeFromUpdate(domain.Plugin{
		ID:               "WFX:c",
		Name:             "CudaLister",
		Path:             filepath.Join(pluginDir, "cuda.wlx"),
		InstalledVersion: "1.8",
		LatestVersion:    "1.9",
	}, kept, []string{"cuda.wlx", "added-by-the-update.txt"})); err != nil {
		t.Fatal(err)
	}

	// What the panel acts on is the line it was given, copy and all.
	hist, err := app.GetUpdateHistory()
	if err != nil || len(hist) != 1 {
		t.Fatalf("GetUpdateHistory: %v (%d lines)", err, len(hist))
	}
	if hist[0].BackupPath != kept {
		t.Fatalf("the last change to a plugin offers no copy: %q", hist[0].BackupPath)
	}

	result, err := app.RollbackUpdate(hist[0])
	if err != nil || result.Locked {
		t.Fatalf("RollbackUpdate: %v (locked=%v)", err, result.Locked)
	}

	got, err := os.ReadFile(filepath.Join(pluginDir, "cuda.wlx"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "the older build" {
		t.Errorf("plugin file = %q, want the kept copy back", got)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "added-by-the-update.txt")); !os.IsNotExist(err) {
		t.Errorf("the file the change named survived the rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "CudaLister.ini")); err != nil {
		t.Errorf("the plugin's own settings file was deleted: %v", err)
	}
	// What the rollback displaced is on disk, even though no line offers it: the
	// way back from here is to update the plugin again.
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Errorf("the version the rollback replaced is not kept: %v", err)
	}

	hist, err = app.GetUpdateHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want the rollback logged too", len(hist))
	}
	if line := hist[0]; !line.Restored || line.Version != "1.8" {
		t.Errorf("newest line = %+v, want a restore of 1.8", line)
	}
	for i, line := range hist {
		if line.BackupPath != "" {
			t.Errorf("line %d still offers a rollback: %+v", i, line)
		}
	}
}

// A second window, or an edited log, must not be able to roll back a change the
// folder has already moved past — including the rollback that moved it.
func TestRollbackUpdateRefusesAnyLineButTheLast(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	app := NewApp()

	root := t.TempDir()
	writeGuiFile(t, filepath.Join(root, "wincmd.exe"), "exe")
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeGuiFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")

	kept := filepath.Join(config.BackupSpacePath(), "wfx", "CudaLister.bak_1700000000")
	writeGuiFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	if err := app.db.RecordChange(storage.ChangeFromUpdate(domain.Plugin{
		ID:               "WFX:c",
		Name:             "CudaLister",
		Path:             filepath.Join(pluginDir, "cuda.wlx"),
		InstalledVersion: "1.8",
		LatestVersion:    "1.9",
	}, kept, nil)); err != nil {
		t.Fatal(err)
	}
	hist, err := app.GetUpdateHistory()
	if err != nil || len(hist) != 1 {
		t.Fatalf("GetUpdateHistory: %v (%d lines)", err, len(hist))
	}
	if _, err := app.RollbackUpdate(hist[0]); err != nil {
		t.Fatalf("RollbackUpdate: %v", err)
	}

	if _, err := app.RollbackUpdate(hist[0]); err == nil {
		t.Error("the same line rolled back twice: the folder had moved past it")
	}
	// And the line the rollback logged offers nothing either.
	hist, err = app.GetUpdateHistory()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RollbackUpdate(hist[0]); err == nil {
		t.Error("a rollback line can be rolled back, which is what the update button is for")
	}
}

// Only the newest line about a plugin describes the folder as it stands, so the
// older ones keep nothing, and a plugin nobody has rolled back keeps its copy.
func TestGetUpdateHistoryOffersOnlyTheLastChange(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())
	app := NewApp()

	first := filepath.Join(t.TempDir(), "CudaLister.bak_1700000000")
	second := filepath.Join(t.TempDir(), "CudaLister.bak_1800000000")
	writeGuiFile(t, filepath.Join(first, "cuda.wlx"), "oldest")
	writeGuiFile(t, filepath.Join(second, "cuda.wlx"), "older")

	for _, seeded := range []struct {
		version string
		backup  string
	}{
		{"1.0", first},
		{"2.0", second},
	} {
		if err := app.db.RecordChange(storage.AppliedChange{
			PluginID:        "WFX:c",
			PluginName:      "CudaLister",
			Version:         seeded.version,
			ReplacedVersion: "whatever was before",
			BackupPath:      seeded.backup,
		}); err != nil {
			t.Fatal(err)
		}
	}

	hist, err := app.GetUpdateHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	if hist[0].BackupPath != second {
		t.Errorf("the last change lost its copy: %q", hist[0].BackupPath)
	}
	if hist[1].BackupPath != "" {
		t.Errorf("an update the folder moved past still offers a rollback: %q", hist[1].BackupPath)
	}
}
