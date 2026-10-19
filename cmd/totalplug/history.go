package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/goloveshko/totalplug/internal/domain"
	"github.com/goloveshko/totalplug/internal/rollback"
	"github.com/goloveshko/totalplug/internal/storage"
	"github.com/goloveshko/totalplug/internal/updater"
)

// printHistory lists the changes the updater applied, newest first. Only the
// newest line about a plugin still names a copy the folder can go back to, so
// only that one is offered for a rollback; the rule is the one the GUI panel
// shows over the same log.
func printHistory(db *storage.Database) {
	entries := db.UpdateHistory()
	rollback.ClearStaleCopies(entries)
	if len(entries) == 0 {
		fmt.Println("[i] Nothing has been updated by TotalPlug yet, so the history is empty.")
		return
	}

	fmt.Printf("\nUpdate history (%d line(s), newest first):\n\n", len(entries))
	for i, e := range entries {
		kind := "update"
		if e.Restored {
			kind = "rollback"
		}
		from := e.ReplacedVersion
		if from == "" {
			from = "unknown"
		}
		fmt.Printf("%2d. %s  %-8s %-20s %s -> %s\n",
			i+1, e.AppliedAt.Local().Format("2006-01-02 15:04"), kind, truncateString(e.PluginName, 20), from, e.Version)
		if e.BackupPath != "" {
			fmt.Printf("    go back with: totalplug --rollback %q\n", e.PluginName)
		}
	}
}

// historyMatch returns the newest line about the plugin the argument names: by
// catalog id, or by the name the history logged.
func historyMatch(entries []domain.UpdateEntry, key string) (domain.UpdateEntry, bool) {
	for _, e := range entries {
		if e.PluginID == key || strings.EqualFold(e.PluginName, key) {
			return e, true
		}
	}
	return domain.UpdateEntry{}, false
}

// runRollback reads the log, puts one plugin's kept copy back and records the
// change. It needs no scan: the line names both the folder and the copy, and the
// updater checks each of them against what is on disk before writing.
func runRollback(db *storage.Database, key string, in io.Reader) error {
	entries := db.UpdateHistory()
	rollback.ClearStaleCopies(entries)

	entry, found := historyMatch(entries, key)
	if !found {
		return fmt.Errorf("nothing in the update history matches %q; run --history to see it", key)
	}
	if entry.BackupPath == "" {
		if entry.Restored {
			return fmt.Errorf("the last change to %s was already a rollback; to get the newer version back, update it again", entry.PluginName)
		}
		return fmt.Errorf("the last change to %s keeps no copy that still exists; retention has taken it away", entry.PluginName)
	}

	fmt.Printf("\nRolling back the last change to %s:\n", entry.PluginName)
	fmt.Printf("  folder:  %s\n", entry.PluginDir)
	fmt.Printf("  go back: %s -> %s\n", entry.Version, entry.ReplacedVersion)
	fmt.Printf("  copy:    %s\n", entry.BackupPath)
	if !confirm(in, "\n[?] Apply this rollback? [y/N]: ") {
		fmt.Println("[i] Rollback skipped.")
		return nil
	}

	outcome, err := rollback.Apply(updater.NewUpdater(), db, entry)
	if err != nil {
		if errors.Is(err, updater.ErrFileLocked) {
			fmt.Println("      ↳ Files are locked or read-only. Close Total Commander and run again.")
		}
		return err
	}

	if err := db.RecordChange(storage.ChangeFromRestore(entry, outcome.SnapshotPath)); err != nil {
		fmt.Printf("[!] Could not log the rollback of %s: %v\n", entry.PluginID, err)
	}
	for _, name := range outcome.LeftBehind {
		fmt.Printf("[!] Could not take %s out of %s: it is still held open\n", name, entry.PluginDir)
	}
	fmt.Printf("  [✓] %-20s back to %s (the version it replaced is kept in %s)\n",
		truncateString(entry.PluginName, 20), entry.ReplacedVersion, outcome.SnapshotPath)
	return nil
}
