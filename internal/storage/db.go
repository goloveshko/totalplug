package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	_ = db.Load()
	_ = db.LoadOverrides()
	return db, nil
}

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

func (db *Database) LoadOverrides() error {
	file, err := os.Open(db.overridesPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Pre-populate CudaLister default override for testing
			db.overrides["WLX_CudaLister"] = domain.Override{
				CustomSource: "github:Alexey-T/CudaLister",
			}
			_ = db.SaveOverrides()
			return nil
		}
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(&db.overrides)
}

func (db *Database) SaveOverrides() error {
	file, err := os.Create(db.overridesPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(db.overrides)
}

func (db *Database) SetUserOverride(pluginID string, override domain.Override) error {
	db.overrides[pluginID] = override
	return db.SaveOverrides()
}

// ApplyOverrides applies user-defined names, versions, and sources.
func (db *Database) ApplyOverrides(plugins []domain.Plugin) []domain.Plugin {
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