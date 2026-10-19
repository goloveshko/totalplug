// Package rollback turns one history line back into the folder it describes.
// The GUI panel and the command line both act through it, so the rule about
// which line may still be rolled back has one answer in one place.
package rollback

import (
	"fmt"

	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/storage"
	"github.com/goloveshko/totalplug/internal/updater"
)

// ClearStaleCopies drops, in place, the copy each line of a newest-first
// history names when going back to it would not lead to the folder as it
// stands: only the newest line about a plugin still describes that folder, and
// a line written by a rollback has no rollback of its own — updating again is
// the way forward from it. Retention takes copies away on its own schedule, so
// a copy that is gone is cleared here too.
func ClearStaleCopies(entries []domain.UpdateEntry) {
	seen := make(map[string]bool, len(entries))
	for i, entry := range entries {
		if entry.BackupPath == "" {
			continue
		}
		last := !seen[entry.PluginID]
		seen[entry.PluginID] = true
		if !last || entry.Restored || !updater.BackupStillKept(entry.BackupPath) {
			entries[i].BackupPath = ""
		}
	}
}

// Apply puts the copy one history line kept back into the plugin folder it came
// from.
//
// The line is whatever the caller read out of the log, so it is checked against
// the log again here: a second window, or an edited JSON file, must not be able
// to reach past the newest change to that folder. The caller then records the
// change with storage.ChangeFromRestore; how each interface reports the folders
// afterwards is its own business.
func Apply(u *updater.Updater, db *storage.Database, entry domain.UpdateEntry) (updater.RestoreOutcome, error) {
	newest, found := latest(db, entry.PluginID)
	if !found || newest.Restored || !newest.AppliedAt.Equal(entry.AppliedAt) {
		return updater.RestoreOutcome{}, fmt.Errorf("only the last change to %s can be rolled back", entry.PluginName)
	}
	return u.RestoreEntry(entry)
}

// latest returns the newest line about one plugin in a newest-first log.
func latest(db *storage.Database, pluginID string) (domain.UpdateEntry, bool) {
	if pluginID == "" {
		return domain.UpdateEntry{}, false
	}
	for _, logged := range db.UpdateHistory() {
		if logged.PluginID == pluginID {
			return logged, true
		}
	}
	return domain.UpdateEntry{}, false
}
