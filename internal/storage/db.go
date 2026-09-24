package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"totalplug/internal/domain"
)

type Record struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	FileHash  string    `json:"file_hash,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Database struct {
	// mu guards records/overrides: background GUI scans read them while
	// SaveOverride writes.
	mu            sync.RWMutex
	filePath      string
	overridesPath string
	records       map[string]Record
	overrides     map[string]domain.Override
}

func GetDefaultDbPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	dir := filepath.Join(appData, "TotalPlug")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "installed_plugins.json")
}

func GetDefaultOverridesPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	dir := filepath.Join(appData, "TotalPlug")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "user_overrides.json")
}

func NewDatabase(dbPath string) (*Database, error) {
	if dbPath == "" {
		dbPath = GetDefaultDbPath()
	}

	db := &Database{
		filePath:      dbPath,
		overridesPath: GetDefaultOverridesPath(),
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

// saveOverridesLocked serializes the current overrides map; callers must hold mu.
func (db *Database) saveOverridesLocked() error {
	file, err := os.Create(db.overridesPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(db.overrides)
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
	file, err := os.Create(db.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(db.records)
}

// RememberVersion stores the version applied by a successful update so later
// scans start from a known value instead of "Unknown". It seeds the update
// history in installed_plugins.json.
func (db *Database) RememberVersion(pluginID, version string) error {
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
	return db.saveRecordsLocked()
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