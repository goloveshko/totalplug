package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

type Record struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Database struct {
	// mu guards records/overrides/history: background GUI scans read them
	// while SaveOverride writes.
	mu            sync.RWMutex
	filePath      string
	overridesPath string
	historyPath   string
	records       map[string]Record
	overrides     map[string]domain.Override
	history       []domain.UpdateEntry
}

// MaxHistoryEntries caps the append-only update log so the JSON file cannot
// grow without bound on a long-lived install.
const MaxHistoryEntries = 200

// GetDefaultDbPath points at the data directory, which is next to the
// executable for a portable install and %APPDATA%\totalplug otherwise.
func GetDefaultDbPath() string {
	return filepath.Join(config.DataDir(), "installed_plugins.json")
}

func GetDefaultOverridesPath() string {
	return filepath.Join(config.DataDir(), "user_overrides.json")
}

func NewDatabase(dbPath string) (*Database, error) {
	if dbPath == "" {
		dbPath = GetDefaultDbPath()
	}

	db := &Database{
		filePath:      dbPath,
		overridesPath: GetDefaultOverridesPath(),
		historyPath:   filepath.Join(filepath.Dir(dbPath), "update_history.json"),
		records:       make(map[string]Record),
		overrides:     make(map[string]domain.Override),
	}

	var errs []error
	if err := db.Load(); err != nil {
		errs = append(errs, err)
	}
	if err := db.LoadOverrides(); err != nil {
		errs = append(errs, err)
	}
	if err := db.LoadHistory(); err != nil {
		errs = append(errs, err)
	}
	return db, errors.Join(errs...)
}

// readJSONFile decodes a JSON object into target. A missing file is not an
// error. A file that exists but cannot be decoded is quarantined by renaming it
// to <path>.corrupt so the (now empty) in-memory state can be rewritten cleanly
// without silently clobbering the user's data.
//
// A file holding the JSON value null decodes without an error and leaves a map
// target nil. Writing to a nil map panics, and the window that called it dies
// with it, so the nil is turned into an empty map here.
func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	if decErr := json.Unmarshal(data, target); decErr != nil {
		quarantine := path + ".corrupt"
		_ = os.Rename(path, quarantine)
		return fmt.Errorf("invalid JSON in %s (moved to %s): %w", path, quarantine, decErr)
	}

	switch v := target.(type) {
	case *map[string]Record:
		if *v == nil {
			*v = map[string]Record{}
		}
	case *map[string]domain.Override:
		if *v == nil {
			*v = map[string]domain.Override{}
		}
	}
	return nil
}

func (db *Database) Load() error {
	return readJSONFile(db.filePath, &db.records)
}

func (db *Database) LoadOverrides() error {
	return readJSONFile(db.overridesPath, &db.overrides)
}

func (db *Database) LoadHistory() error {
	return readJSONFile(db.historyPath, &db.history)
}

// saveHistoryLocked serializes the update log; callers must hold mu.
func (db *Database) saveHistoryLocked() error {
	return writeJSONAtomic(db.historyPath, db.history)
}

// saveOverridesLocked serializes the current overrides map; callers must hold mu.
func (db *Database) saveOverridesLocked() error {
	return writeJSONAtomic(db.overridesPath, db.overrides)
}

func (db *Database) SetUserOverride(pluginID string, override domain.Override) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.overrides[pluginID] = override
	return db.saveOverridesLocked()
}

// saveRecordsLocked serializes the current records map; callers must hold mu.
func (db *Database) saveRecordsLocked() error {
	return writeJSONAtomic(db.filePath, db.records)
}

// writeJSONAtomic encodes value into a sibling temp file and renames it over
// path. Writing straight to the target with os.Create truncates it before a
// single byte of the new content exists, so a crash or a full disk there leaves
// a half-written file behind - and the next start would quarantine that as
// .corrupt and save the empty in-memory state over it, which loses records the
// user never touched. Same shape as checker.saveCacheAtomically.
func writeJSONAtomic(path string, value any) error {
	// A temp name of its own for every writer. A fixed one was shared by
	// everyone writing this file at the same moment — a GUI window and a CLI
	// scan — and the loser of that race either failed its rename or renamed a
	// half-written file over the good one.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// AppliedChange is one change the updater applied, as the history log records it.
type AppliedChange struct {
	PluginID        string
	PluginName      string
	Version         string
	ReplacedVersion string
	PluginDir       string
	BackupPath      string
	AppliedFiles    []string
	Restored        bool
}

// ChangeFromUpdate describes a successful update the way RecordChange wants it:
// the folder that changed, the copy the updater left behind, the version that
// copy holds and the files the update wrote. Both the CLI and the GUI apply an
// update and then log it, and the mapping is the same answer in both places.
func ChangeFromUpdate(plugin domain.Plugin, backupPath string, applied []string) AppliedChange {
	return AppliedChange{
		PluginID:        plugin.ID,
		PluginName:      plugin.Name,
		Version:         plugin.LatestVersion,
		ReplacedVersion: plugin.InstalledVersion,
		PluginDir:       filepath.Dir(plugin.Path),
		BackupPath:      backupPath,
		AppliedFiles:    applied,
	}
}

// ChangeFromRestore describes a rollback of one logged change: the version now in
// the folder is the one the kept copy held, and the snapshot taken of what was
// there until a moment ago is the copy that makes this line undoable in turn. It
// carries no file list: a rollback line is never itself rolled back (the way
// forward is to update again), so there is nothing for the list to be asked about.
func ChangeFromRestore(entry domain.UpdateEntry, snapshotPath string) AppliedChange {
	return AppliedChange{
		PluginID:        entry.PluginID,
		PluginName:      entry.PluginName,
		Version:         entry.ReplacedVersion,
		ReplacedVersion: entry.Version,
		PluginDir:       entry.PluginDir,
		BackupPath:      snapshotPath,
		Restored:        true,
	}
}

// RecordChange stores the version applied by a successful update so later scans
// start from a known value instead of "Unknown", and appends the event to the
// capped update history log. A stale manual CustomVersion written before the
// update would otherwise shadow the new record in ApplyOverrides, so it is
// cleared here (CustomName and CustomSource are kept).
//
// An update that replaced a version the scanner never read has nothing to
// report as the new one, so it is logged without a version and leaves the
// remembered record alone — the next scan is what fills it in.
func (db *Database) RecordChange(change AppliedChange) error {
	if change.PluginID == "" || (change.Version == "" && !change.Restored) {
		return nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if change.Version != "" {
		db.records[change.PluginID] = Record{
			ID:        change.PluginID,
			Version:   change.Version,
			UpdatedAt: time.Now().UTC(),
		}
		if err := db.saveRecordsLocked(); err != nil {
			return err
		}
	}

	overridesChanged := false
	if ov, ok := db.overrides[change.PluginID]; ok && ov.CustomVersion != "" {
		ov.CustomVersion = ""
		if ov.CustomName == "" && ov.CustomSource == "" {
			delete(db.overrides, change.PluginID)
		} else {
			db.overrides[change.PluginID] = ov
		}
		overridesChanged = true
	}

	db.history = append([]domain.UpdateEntry{{
		PluginID:        change.PluginID,
		PluginName:      change.PluginName,
		Version:         change.Version,
		AppliedAt:       time.Now().UTC(),
		PluginDir:       change.PluginDir,
		BackupPath:      change.BackupPath,
		ReplacedVersion: change.ReplacedVersion,
		AppliedFiles:    change.AppliedFiles,
		Restored:        change.Restored,
	}}, db.history...)
	if len(db.history) > MaxHistoryEntries {
		db.history = db.history[:MaxHistoryEntries]
	}
	if err := db.saveHistoryLocked(); err != nil {
		return err
	}
	if overridesChanged {
		return db.saveOverridesLocked()
	}
	return nil
}

// UpdateHistory returns the applied-update log, newest entry first.
func (db *Database) UpdateHistory() []domain.UpdateEntry {
	db.mu.RLock()
	defer db.mu.RUnlock()

	out := make([]domain.UpdateEntry, len(db.history))
	copy(out, db.history)
	return out
}

// MigrateLegacyIDs rekeys records and user overrides stored under the old
// "<TYPE>_<name>" identity scheme to the current stable IDs. mapping holds
// legacyID -> newID for unambiguous plugins only; the pipeline drops shared
// legacy IDs so user data is never re-assigned to the wrong plugin. Entries
// whose target key already exists are left untouched. Returns the number of
// moved entries; files are rewritten only when something actually changed.
func (db *Database) MigrateLegacyIDs(mapping map[string]string) (int, error) {
	if len(mapping) == 0 {
		return 0, nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	moved := 0
	recordsChanged, overridesChanged := false, false

	for legacy, next := range mapping {
		if legacy == "" || next == "" || legacy == next {
			continue
		}
		if rec, ok := db.records[legacy]; ok {
			if _, exists := db.records[next]; !exists {
				rec.ID = next
				db.records[next] = rec
				delete(db.records, legacy)
				recordsChanged = true
				moved++
			}
		}
		if ov, ok := db.overrides[legacy]; ok {
			if _, exists := db.overrides[next]; !exists {
				db.overrides[next] = ov
				delete(db.overrides, legacy)
				overridesChanged = true
				moved++
			}
		}
	}

	if recordsChanged {
		if err := db.saveRecordsLocked(); err != nil {
			return moved, fmt.Errorf("saving migrated records: %w", err)
		}
	}
	if overridesChanged {
		if err := db.saveOverridesLocked(); err != nil {
			return moved, fmt.Errorf("saving migrated overrides: %w", err)
		}
	}
	return moved, nil
}

// ApplyOverrides applies user-defined names, versions, and sources.
func (db *Database) ApplyOverrides(plugins []domain.Plugin) []domain.Plugin {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for i := range plugins {
		p := &plugins[i]

		// Apply DB recorded version first
		if rec, exists := db.records[p.ID]; exists && rec.Version != "" {
			p.InstalledVersion = rec.Version
			p.Confidence = domain.ConfidenceManual
		}

		// Apply User Manual Overrides
		if ov, exists := db.overrides[p.ID]; exists {
			if ov.CustomName != "" {
				p.Name = ov.CustomName
			}
			if ov.CustomVersion != "" {
				p.InstalledVersion = ov.CustomVersion
				p.Confidence = domain.ConfidenceManual
			}
			if ov.CustomSource != "" {
				p.CustomSource = ov.CustomSource
			}
		}
	}
	return plugins
}
