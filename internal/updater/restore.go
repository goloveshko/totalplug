package updater

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

// RestoreEntry puts the copy one history line kept back into the plugin folder it
// came from, and returns a copy of the version that was installed until now.
// Nothing offers rolling a rollback back — updating again is the way forward — but
// that second copy is what makes the answer hold even after the catalog moved on.
//
// The folder is taken away before the copy is written back instead of being
// merged into: the files a newer version added have no counterpart in the copy,
// so a merge would leave them sitting in a plugin that is supposed to be the
// older one again.
func (u *Updater) RestoreEntry(entry domain.UpdateEntry) (string, error) {
	backupDir, err := restoreSource(entry)
	if err != nil {
		return "", err
	}
	pluginDir := entry.PluginDir

	snapshot, err := u.BackupPlugin(pluginDir)
	if err != nil {
		return "", fmt.Errorf("backing up the version being replaced: %w", err)
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return snapshot, asLocked(fmt.Errorf("clearing %s: %w", pluginDir, err))
	}
	if err := copyTree(backupDir, pluginDir); err != nil {
		// The folder now holds part of one version, so put back the whole of what
		// the snapshot held rather than leaving the plugin in neither.
		if again := copyTree(snapshot, pluginDir); again != nil {
			return snapshot, fmt.Errorf("restoring %s: %v; what was there before is in %s and has to be put back by hand: %v",
				backupDir, err, snapshot, again)
		}
		return snapshot, asLocked(fmt.Errorf("restoring %s: %w", backupDir, err))
	}

	// Best-effort, exactly as after an update: the rollback already succeeded, so
	// old copies must not turn it into a reported failure.
	_, _ = pruneBackupSpace(pluginDir, config.LoadBackupPolicy())
	return snapshot, nil
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
	// A rollback deletes its target, so the folder has to be inside the install
	// this tool was pointed at; a stale or edited path must not be a way to empty
	// some other part of the disk.
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
