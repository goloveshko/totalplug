package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/goloveshko/totalplug/internal/domain"
)

// tcFixture builds a Total Commander install — one wincmd.exe is what makes a
// folder that — and returns its root.
func tcFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "wincmd.exe"), "exe")
	return root
}

func TestRestoreEntryPutsTheKeptCopyBack(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	// A file only the newer version has: nothing in the copy can overwrite it, so
	// a rollback that merges would leave it behind in an older plugin.
	writeTestFile(t, filepath.Join(pluginDir, "added-by-the-update.txt"), "extra")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	snapshot, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:        "WFX:c",
		PluginName:      "CudaLister",
		Version:         "1.9",
		ReplacedVersion: "1.8",
		PluginDir:       pluginDir,
		BackupPath:      kept,
	})
	if err != nil {
		t.Fatalf("RestoreEntry: %v", err)
	}

	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the older build" {
		t.Errorf("plugin file = %q, want the copy put back", got)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "added-by-the-update.txt")); !os.IsNotExist(err) {
		t.Errorf("a file only the newer version had survived the rollback: %v", err)
	}

	// The version that was just removed has to be recoverable by the same route,
	// which means the snapshot is a copy of its own in the same namespace.
	if _, ok := backupStamp(filepath.Base(snapshot), pluginDir); !ok {
		t.Fatalf("snapshot %q is not a copy of %s", snapshot, pluginDir)
	}
	if got := readTestFile(t, filepath.Join(snapshot, "cuda.wlx")); got != "the newer build" {
		t.Errorf("snapshot file = %q, want the version the rollback replaced", got)
	}
}

func TestRestoreEntryRefusesACopyOfAnotherPlugin(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "mine")

	space := backupSpaceFor(t, pluginDir)
	// The right shape, in the right folder, but it belongs to a different plugin
	// sharing the same type folder.
	other := filepath.Join(space, "ACDCommand.bak_1001")
	writeTestFile(t, filepath.Join(other, "acd.wcx"), "someone else's")

	_, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:   "WFX:c",
		PluginDir:  pluginDir,
		BackupPath: other,
	})
	if !errors.Is(err, ErrBackupGone) {
		t.Fatalf("RestoreEntry error = %v, want ErrBackupGone", err)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "mine" {
		t.Errorf("plugin file = %q after a refused rollback", got)
	}
}

func TestRestoreEntryRefusesAFolderOutsideAnInstall(t *testing.T) {
	// A history line is a path from a JSON file. Deleting whatever it names is not
	// something to do without checking that it is inside Total Commander.
	pluginDir := filepath.Join(t.TempDir(), "somewhere", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "mine")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "older")

	if _, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:   "WFX:c",
		PluginDir:  pluginDir,
		BackupPath: kept,
	}); !errors.Is(err, ErrPluginOutsideTCRoot) {
		t.Fatalf("RestoreEntry error = %v, want ErrPluginOutsideTCRoot", err)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "mine" {
		t.Errorf("plugin file = %q after a refused rollback", got)
	}
}

func TestRestoreEntryRefusesACopyRetentionDeleted(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "mine")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "older")
	if err := os.RemoveAll(kept); err != nil {
		t.Fatal(err)
	}

	if _, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:   "WFX:c",
		PluginDir:  pluginDir,
		BackupPath: kept,
	}); !errors.Is(err, ErrBackupGone) {
		t.Fatalf("RestoreEntry error = %v, want ErrBackupGone", err)
	}
}

func TestRestoreEntryRefusesAnEntryWithoutACopy(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "mine")

	if _, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:  "WFX:c",
		PluginDir: pluginDir,
	}); !errors.Is(err, ErrBackupGone) {
		t.Fatalf("RestoreEntry error = %v, want ErrBackupGone", err)
	}
}
