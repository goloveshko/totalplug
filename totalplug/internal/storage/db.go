package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"totalplug/internal/domain"
)

// Record represents local metadata for a single plugin stored on disk.
type Record struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	FileHash  string    `json:"file_hash,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Database handles reading and writing TotalPlug local state.
type Database struct {
	filePath string
	records  map[string]Record
}

// GetDefaultDbPath returns path to %APPDATA%\TotalPlug\installed_plugins.json
func GetDefaultDbPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = "."
	}
	dir := filepath.Join(appData, "TotalPlug")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "installed_plugins.json")
}

// NewDatabase initializes a new Database instance.
func NewDatabase(dbPath string) (*Database, error) {
	if dbPath == "" {
		dbPath = GetDefaultDbPath()
	}

	db := &Database{
		filePath: dbPath,
		records:  make(map[string]Record),
	}

	_ = db.Load()
	return db, nil
}

// Load reads records from JSON file on disk.
func (db *Database) Load() error {
	file, err := os.Open(db.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(&db.records)
}

// Save writes records to JSON file on disk.
func (db *Database) Save() error {
	file, err := os.Create(db.filePath)
	if err != nil {
		return fmt.Errorf("failed to save database: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(db.records)
}

// SavePluginRecord records or updates a plugin entry in the database.
func (db *Database) SavePluginRecord(pluginID, version, hash string) error {
	db.records[pluginID] = Record{
		ID:        pluginID,
		Version:   version,
		FileHash:  hash,
		UpdatedAt: time.Now(),
	}
	return db.Save()
}

// EnrichFromDatabase overrides uncertain plugin versions if a recorded version exists in DB.
func (db *Database) EnrichFromDatabase(plugins []domain.Plugin) []domain.Plugin {
	for i := range plugins {
		rec, exists := db.records[plugins[i].ID]
		if exists && rec.Version != "" {
			// If version was low confidence, override with DB recorded version
			if plugins[i].Confidence == domain.ConfidenceFallback || plugins[i].Confidence == domain.ConfidenceText {
				plugins[i].InstalledVersion = rec.Version
				plugins[i].Confidence = domain.ConfidenceRecorded
				plugins[i].Status = domain.StatusUpToDate
			}
			plugins[i].FileHash = rec.FileHash
		}
	}
	return plugins
}
