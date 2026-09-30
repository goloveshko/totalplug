package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"totalplug/internal/config"
	"totalplug/internal/domain"
)

type Record struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	FileHash  string    `json:"file_hash,omitempty"`
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

func (db *Database) SaveOverrides() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.saveOverridesLocked()
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
	tmpFile := path + ".tmp"

	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmpFile)
		return err
	}
	return os.Rename(tmpFile, path)
}

// RememberVersion stores the version applied by a successful update so later
// scans start from a known value instead of "Unknown", and appends the event
// to the capped update history log. A stale manual CustomVersion written
// before the update would otherwise shadow the new record in ApplyOverrides,
// so it is cleared here (CustomName and CustomSource are kept).
func (db *Database) RememberVersion(pluginID, pluginName, version string) error {
	if pluginID == "" || version == "" {
		return nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.records[pluginID] = Record{
		ID:        pluginID,
		Version:   version,
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.saveRecordsLocked(); err != nil {
		return err
	}

	overridesChanged := false
	if ov, ok := db.overrides[pluginID]; ok && ov.CustomVersion != "" {
		ov.CustomVersion = ""
		if ov.CustomName == "" && ov.CustomSource == "" {
			delete(db.overrides, pluginID)
		} else {
			db.overrides[pluginID] = ov
		}
		overridesChanged = true
	}

	db.history = append([]domain.UpdateEntry{{
		PluginID:   pluginID,
		PluginName: pluginName,
		Version:    version,
		AppliedAt:  time.Now().UTC(),
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
