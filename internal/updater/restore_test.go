package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	// The three kinds of file a plugin folder holds besides the version itself:
	// one the update brought, one the plugin wrote for its own settings, and one
	// the user put there. Only the first is this tool's to take away.
	writeTestFile(t, filepath.Join(pluginDir, "added-by-the-update.txt"), "extra")
	writeTestFile(t, filepath.Join(pluginDir, "CudaLister.ini"), "the settings the newer build saved")
	writeTestFile(t, filepath.Join(pluginDir, "notes.txt"), "a file the user placed here")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	outcome, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:        "WFX:c",
		PluginName:      "CudaLister",
		Version:         "1.9",
		ReplacedVersion: "1.8",
		PluginDir:       pluginDir,
		BackupPath:      kept,
		AppliedFiles:    []string{"cuda.wlx", "added-by-the-update.txt"},
	})
	if err != nil {
		t.Fatalf("RestoreEntry: %v", err)
	}

	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the older build" {
		t.Errorf("plugin file = %q, want the copy put back", got)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "added-by-the-update.txt")); !os.IsNotExist(err) {
		t.Errorf("a file the update named survived the rollback: %v", err)
	}
	for _, name := range []string{"CudaLister.ini", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(pluginDir, name)); err != nil {
			t.Errorf("%s was not in the update's list and should have been left alone: %v", name, err)
		}
	}
	if outcome.LeftBehind != nil {
		t.Errorf("LeftBehind = %v, want nothing left behind", outcome.LeftBehind)
	}

	// The version that was just removed has to be recoverable by the same route,
	// which means the snapshot is a copy of its own in the same namespace.
	if _, ok := backupStamp(filepath.Base(outcome.SnapshotPath), pluginDir); !ok {
		t.Fatalf("snapshot %q is not a copy of %s", outcome.SnapshotPath, pluginDir)
	}
	if got := readTestFile(t, filepath.Join(outcome.SnapshotPath, "cuda.wlx")); got != "the newer build" {
		t.Errorf("snapshot file = %q, want the version the rollback replaced", got)
	}
}

func TestRollbackOfAnEntryWithoutAFileListRemovesNothing(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	writeTestFile(t, filepath.Join(pluginDir, "added-by-the-update.txt"), "extra")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	// History lines written before the tool recorded names carry an empty list.
	// With nothing to go by, the only safe reading of "take away what the update
	// added" is to take away nothing: the older plugin keeps an extra file it does
	// not know about, which beats deleting someone's settings.
	if _, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:   "WFX:c",
		PluginDir:  pluginDir,
		BackupPath: kept,
	}); err != nil {
		t.Fatalf("RestoreEntry: %v", err)
	}

	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the older build" {
		t.Errorf("plugin file = %q, want the copy put back", got)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "added-by-the-update.txt")); got != "extra" {
		t.Errorf("an extra file was deleted on the strength of no list: %q", got)
	}
}

func TestRollbackKeepsThePluginFolder(t *testing.T) {
	// Total Commander holds a loaded plugin as an image section, and while the
	// file itself can be renamed over, the folder around it cannot be removed —
	// so a rollback that emptied the folder first needed the program closed.
	// Nothing here removes it, which is what makes rolling back right after an
	// update work.
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	writeTestFile(t, filepath.Join(pluginDir, "descript", "readme.txt"), "the newer build brought this")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	if _, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:     "WFX:c",
		PluginDir:    pluginDir,
		BackupPath:   kept,
		AppliedFiles: []string{"cuda.wlx", filepath.Join("descript", "readme.txt")},
	}); err != nil {
		t.Fatalf("RestoreEntry: %v", err)
	}

	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the older build" {
		t.Errorf("plugin file = %q, want the copy put back", got)
	}
	// Every file the update brought is gone, yet the folder and the directory it
	// came with are still there: an empty folder is nobody's data, and removing
	// one is exactly what a loaded plugin does not allow.
	info, err := os.Stat(pluginDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("the plugin folder is gone after a rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "descript")); err != nil {
		t.Errorf("the folder an update added was deleted: %v", err)
	}
}

func TestRollbackReportsWhatItCouldNotTakeOut(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	// A folder in the way of the removal: the rollback itself still happens, so
	// this has to come back as a note rather than a failure.
	writeTestFile(t, filepath.Join(pluginDir, "blocker", "inside.txt"), "held")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	outcome, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:     "WFX:c",
		PluginDir:    pluginDir,
		BackupPath:   kept,
		AppliedFiles: []string{"blocker"},
	})
	if err != nil {
		t.Fatalf("a rollback that could not clear one file must still report success: %v", err)
	}
	if strings.Join(outcome.LeftBehind, ",") != "blocker" {
		t.Errorf("LeftBehind = %v, want the name that would not go", outcome.LeftBehind)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the older build" {
		t.Errorf("plugin file = %q, want the copy put back anyway", got)
	}
}

func TestRollbackThatCouldNotWriteKeepsNoCopy(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	// A name the two trees disagree about: the plugin folder has a directory the
	// kept copy holds as a file, so writing the copy cannot replace it and the
	// rollback fails. Whatever the reason the copy could not be written, the
	// folder comes back holding the version it had.
	writeTestFile(t, filepath.Join(pluginDir, "data", "saved.txt"), "the newer settings")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")
	writeTestFile(t, filepath.Join(kept, "data"), "the older settings, saved as a file")

	outcome, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:     "WFX:c",
		PluginDir:    pluginDir,
		BackupPath:   kept,
		AppliedFiles: []string{"cuda.wlx"},
	})
	if err == nil {
		t.Fatal("a copy that cannot replace a directory is expected to fail")
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "cuda.wlx")); got != "the newer build" {
		t.Errorf("plugin file = %q, want the version that was installed put back", got)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "data", "saved.txt")); got != "the newer settings" {
		t.Errorf("the folder the copy collided with was disturbed: %q", got)
	}
	// The snapshot was made for a rollback that did not happen, and no history
	// line can point at it: the line still describes the change in place.
	if outcome.SnapshotPath != "" {
		t.Errorf("SnapshotPath = %q, want the failed run to keep no copy", outcome.SnapshotPath)
	}
	remaining, err := filepath.Glob(filepath.Join(space, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || filepath.Base(remaining[0]) != "CudaLister.bak_1001" {
		t.Errorf("the namespace holds %v, want only the copy the line is still owed", remaining)
	}
}

// Total Commander holds the plugin file it loaded, and a rename cannot replace
// the image it maps. The copy back then gives up at that same file, having
// already put back every file the copy out changed before it, so the folder is
// provably the version it started with: the run has to say so as an ordinary
// lock, keep no copy of a rollback that did not happen, take away the files the
// attempt itself wrote, and leave nothing to rescue by hand.
func TestRollbackStoppedByTheSameFileKeepsNoCopy(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wlx", "fileinfo")
	// Walk order is a.txt, m.txt then z.txt, so the copy out damages a.txt,
	// writes a file the folder never held, and only then meets the one that will
	// not be replaced.
	writeTestFile(t, filepath.Join(pluginDir, "a.txt"), "a-new")
	blocker := filepath.Join(pluginDir, "z.txt")
	writeTestFile(t, blocker, "z-new")
	if err := os.Chmod(blocker, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocker, 0644) })

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "fileinfo.bak_1001")
	writeTestFile(t, filepath.Join(kept, "a.txt"), "a-old")
	writeTestFile(t, filepath.Join(kept, "m.txt"), "m-old")
	writeTestFile(t, filepath.Join(kept, "z.txt"), "z-old")

	outcome, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:     "WLX:f",
		PluginDir:    pluginDir,
		BackupPath:   kept,
		AppliedFiles: []string{"a.txt", "z.txt"},
	})
	if err == nil {
		t.Fatal("expected the held file to stop the rollback")
	}
	if !errors.Is(err, ErrFileLocked) {
		t.Errorf("error should be a locked-file error: %v", err)
	}
	if strings.Contains(err.Error(), "by hand") {
		t.Errorf("nothing needs rescuing by hand, but the run says it does: %v", err)
	}
	if outcome.SnapshotPath != "" {
		t.Errorf("SnapshotPath = %q, want the failed run to keep no copy", outcome.SnapshotPath)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "a.txt")); got != "a-new" {
		t.Errorf("a.txt = %q, want the version that was installed put back", got)
	}
	if got := readTestFile(t, blocker); got != "z-new" {
		t.Errorf("z.txt = %q, want the held file left as it was", got)
	}
	// The copy back had no name for this one: the version being put back never
	// wrote it, because it was not in the folder when that version was installed.
	if _, err := os.Stat(filepath.Join(pluginDir, "m.txt")); !os.IsNotExist(err) {
		t.Errorf("the file this attempt wrote into the folder is still there (err = %v)", err)
	}
	remaining, err := filepath.Glob(filepath.Join(space, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || filepath.Base(remaining[0]) != "fileinfo.bak_1001" {
		t.Errorf("the namespace holds %v, want only the copy the line is still owed", remaining)
	}
}

func TestRollbackIgnoresANameOutsideThePluginFolder(t *testing.T) {
	root := tcFixture(t)
	pluginDir := filepath.Join(root, "wfx", "CudaLister")
	writeTestFile(t, filepath.Join(pluginDir, "cuda.wlx"), "the newer build")
	writeTestFile(t, filepath.Join(root, "wfx", "wanted-by-a-crafted-history-file.txt"), "outside")

	space := backupSpaceFor(t, pluginDir)
	kept := filepath.Join(space, "CudaLister.bak_1001")
	writeTestFile(t, filepath.Join(kept, "cuda.wlx"), "the older build")

	// The list is read back from a JSON file the user can edit, so a name that
	// leads out of the folder is not treated as one this tool wrote there.
	if _, err := NewUpdater().RestoreEntry(domain.UpdateEntry{
		PluginID:   "WFX:c",
		PluginDir:  pluginDir,
		BackupPath: kept,
		AppliedFiles: []string{
			"..\\wfx\\wanted-by-a-crafted-history-file.txt",
			"../wfx/wanted-by-a-crafted-history-file.txt",
		},
	}); err != nil {
		t.Fatalf("RestoreEntry: %v", err)
	}

	if got := readTestFile(t, filepath.Join(root, "wfx", "wanted-by-a-crafted-history-file.txt")); got != "outside" {
		t.Errorf("a rollback deleted outside the plugin folder: %q", got)
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
