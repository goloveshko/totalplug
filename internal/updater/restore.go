package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

// RestoreOutcome is what a rollback leaves: the copy of the version it took away,
// which is what makes the same route lead forward again, and the files the update
// named but this run could not take out of the folder.
type RestoreOutcome struct {
	SnapshotPath string
	LeftBehind   []string
}

// RestoreEntry puts the copy one history line kept back into the plugin folder it
// came from, and returns a copy of the version that was installed until now.
// Nothing offers rolling a rollback back — updating again is the way forward — but
// that second copy is what makes the answer hold even after the catalog moved on.
//
// The copy is written over the folder with the same rename-into-place an update
// uses rather than by emptying the folder first: Total Commander holds a loaded
// plugin, and while replacing its file works, removing the folder around it does
// not — an update has never had to. What the update added on top of the copy is
// then taken away, but only by the names that line recorded, because a plugin
// writes its settings next to its DLL and the folder can hold a file the user put
// there himself. A line written before the tool recorded names removes nothing.
//
// A copy that cannot be written leaves the folder holding the version it had, with
// no second copy kept; the failure that restored the folder also proved the line
// still describes it, so the same rollback can simply be asked again. Files this
// attempt had already written there before it stopped go out again with it, because
// the version being put back has no name for them.
func (u *Updater) RestoreEntry(entry domain.UpdateEntry) (RestoreOutcome, error) {
	backupDir, err := restoreSource(entry)
	if err != nil {
		return RestoreOutcome{}, err
	}
	pluginDir := entry.PluginDir

	snapshot, err := u.BackupPlugin(pluginDir)
	if err != nil {
		return RestoreOutcome{}, fmt.Errorf("backing up the version being replaced: %w", err)
	}

	if failed, err := copyTree(backupDir, pluginDir); err != nil {
		// The folder now holds part of one version, so put back the whole of what
		// the snapshot held rather than leaving the plugin in neither.
		stoppedAt, again := copyTree(snapshot, pluginDir)
		// Writing the snapshot back only replaces the files it holds, so a file
		// this attempt brought into the folder is still standing there. It cannot
		// have been there before the attempt, so it goes out again the same way.
		created := takeOutItsOwnWork(pluginDir, backupDir, snapshot)
		if again != nil && stoppedAt != failed {
			// The copy back gave up somewhere else than the copy out did, so the
			// folder holds part of one version and part of the other, and this
			// snapshot is the only readable whole left.
			return RestoreOutcome{SnapshotPath: snapshot}, fmt.Errorf("restoring %s: %v; what was there before is in %s and has to be put back by hand: %v",
				backupDir, err, snapshot, again)
		}
		// Either the copy back finished, or it gave up at exactly the file that
		// blocked the copy out — which that pass never changed, and whose earlier
		// files the copy back has since rewritten. The folder holds the version it
		// had either way, so the copy made for a rollback that did not happen is a
		// duplicate of what is installed: no line can point at it, since the line
		// still describes the change that is in place, and retrying makes another.
		// Take it away, as an update does with its backup after its own rollback.
		_ = os.RemoveAll(snapshot)
		failure := fmt.Errorf("restoring %s: %w", backupDir, err)
		if len(created) > 0 {
			failure = fmt.Errorf("%w; the folder also still holds %s, written there by this attempt", failure, strings.Join(created, ", "))
		}
		return RestoreOutcome{}, asLocked(failure)
	}

	left := removeAddedFiles(pluginDir, entry.AppliedFiles, backupDir)

	// Best-effort, exactly as after an update: the rollback already succeeded, so
	// old copies must not turn it into a reported failure.
	_, _ = pruneBackupSpace(pluginDir, config.LoadBackupPolicy())
	return RestoreOutcome{SnapshotPath: snapshot, LeftBehind: left}, nil
}

// removeAddedFiles takes out of the folder the files the named update wrote there
// and the copy just restored has no answer to. A name outside the folder is not
// a file this tool added, so it is left alone, and so is anything the list does
// not mention. What will not go — Total Commander can be holding a file the newer
// version brought — comes back in left rather than failing a rollback that did
// happen.
func removeAddedFiles(pluginDir string, applied []string, backupDir string) []string {
	var left []string
	for _, name := range applied {
		target, err := sanitizeEntryPath(pluginDir, name)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(backupDir, filepath.Clean(name))); err == nil {
			continue // the copy restored this one itself, so it belongs to the older version
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			left = append(left, name)
		}
	}
	return left
}

// takeOutItsOwnWork removes the files a rollback attempt wrote into the plugin
// folder before it stopped. The copy being restored holds such a file while the
// snapshot of the version in place does not, which means it was not in the folder
// beforehand and only this attempt can have put it there. Directories are left as
// they are: an empty one says nothing about a version, and a full one is not this
// function's to empty. What will not go comes back named.
func takeOutItsOwnWork(pluginDir, backupDir, snapshot string) []string {
	var left []string
	_ = filepath.Walk(backupDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		rel := relWithin(backupDir, path)
		if _, err := os.Stat(filepath.Join(snapshot, rel)); err == nil {
			return nil
		}
		target, err := sanitizeEntryPath(pluginDir, rel)
		if err != nil {
			return nil
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			left = append(left, rel)
		}
		return nil
	})
	return left
}

// BackupStillKept reports whether a copy named by a history line is still on
// disk. Retention deletes copies on its own schedule and the data folder can be
// cleaned by hand, so this is asked when the line is shown, not when it was
// written.
func BackupStillKept(backupPath string) bool {
	info, err := os.Stat(backupPath)
	return err == nil && info.IsDir()
}

// restoreSource checks the two claims a history line makes about where to put
// what back. The line comes out of a JSON file, so neither is taken on trust: the
// folder it names has to be one this tool may work on, and the copy has to be one
// this tool wrote for that folder.
func restoreSource(entry domain.UpdateEntry) (string, error) {
	if entry.PluginDir == "" || entry.BackupPath == "" {
		return "", fmt.Errorf("%w: the entry does not name a copy to put back", ErrBackupGone)
	}
	if err := validatePluginFolder(entry.PluginDir); err != nil {
		return "", err
	}
	// A rollback writes into its target and takes named files out of it, so the
	// folder has to be inside the install this tool was pointed at; a stale or
	// edited path must not be a way to overwrite some other part of the disk.
	if !pluginInsideTCRoot(entry.PluginDir) {
		return "", fmt.Errorf("%w: %s", ErrPluginOutsideTCRoot, entry.PluginDir)
	}
	if _, ok := backupStamp(filepath.Base(entry.BackupPath), entry.PluginDir); !ok {
		return "", fmt.Errorf("%w: %s is not a copy of %s", ErrBackupGone, entry.BackupPath, entry.PluginDir)
	}
	space, err := backupNamespace(entry.PluginDir)
	if err != nil {
		return "", err
	}
	if filepath.Dir(entry.BackupPath) != space {
		return "", fmt.Errorf("%w: %s is not where the copies of %s live", ErrBackupGone, entry.BackupPath, space)
	}
	if !BackupStillKept(entry.BackupPath) {
		return "", fmt.Errorf("%w: %s", ErrBackupGone, entry.BackupPath)
	}
	return entry.BackupPath, nil
}
