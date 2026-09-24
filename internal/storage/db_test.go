package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"totalplug/internal/domain"
)

// newTestDb points APPDATA at a temp dir so default paths stay hermetic.
func newTestDb(t *testing.T) string {
	t.Helper()
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	dir := filepath.Join(appData, "TotalPlug")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNewDatabaseMissingFilesAreNotErrors(t *testing.T) {
	newTestDb(t)

	db, err := NewDatabase("")
	if err != nil {
		t.Fatalf("missing files must not error, got: %v", err)
	}
	if db == nil {
		t.Fatal("expected usable database handle")
	}
}

func TestNewDatabaseQuarantinesCorruptRecords(t *testing.T) {
	dir := newTestDb(t)
	recordsPath := filepath.Join(dir, "installed_plugins.json")
	if err := os.WriteFile(recordsPath, []byte(`{"broken: [`), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := NewDatabase("")
	if err == nil {
		t.Fatal("corrupt records file must surface an error")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("error should mention invalid JSON, got: %v", err)
	}

	if _, e := os.Stat(recordsPath); !os.IsNotExist(e) {
		t.Error("corrupt file should be renamed away from its original path")
	}
	if _, e := os.Stat(recordsPath + ".corrupt"); e != nil {
		t.Errorf("quarantine file missing: %v", e)
	}
	if db == nil {
		t.Fatal("database must still be usable after quarantine")
	}
}

func TestNewDatabaseJoinsMultipleFailures(t *testing.T) {
	dir := newTestDb(t)
	corrupt := []byte(`{{{{not json`)
	if err := os.WriteFile(filepath.Join(dir, "installed_plugins.json"), corrupt, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user_overrides.json"), corrupt, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewDatabase("")
	if err == nil {
		t.Fatal("expected combined error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "installed_plugins.json") || !strings.Contains(msg, "user_overrides.json") {
		t.Fatalf("joined error should name both files, got: %v", err)
	}
}

func TestOverrideRoundtripThroughDisk(t *testing.T) {
	newTestDb(t)

	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}
	ov := domain.Override{
		CustomName:    "CudaLister",
		CustomVersion: "1.8.6.3",
		CustomSource:  "github:Alexey-T/CudaLister",
	}
	if err := db.SetUserOverride("ltx_CudaLister", ov); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewDatabase("")
	if err != nil {
		t.Fatalf("reloading freshly written overrides failed: %v", err)
	}

	plugins := []domain.Plugin{{ID: "ltx_CudaLister", Name: "old", InstalledVersion: "Unknown"}}
	plugins = reloaded.ApplyOverrides(plugins)
	got := plugins[0]
	if got.Name != ov.CustomName || got.InstalledVersion != ov.CustomVersion || got.CustomSource != ov.CustomSource {
		t.Fatalf("override not applied: %+v", got)
	}
	if got.Confidence != domain.ConfidenceManual {
		t.Fatalf("manual override should set confidence, got %q", got.Confidence)
	}

	// On-disk file must be valid JSON.
	data, err := os.ReadFile(GetDefaultOverridesPath())
	if err != nil {
		t.Fatal(err)
	}
	var check map[string]domain.Override
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("saved overrides are invalid JSON: %v", err)
	}
}

func TestRememberVersionRoundtripAppliesAsManual(t *testing.T) {
	newTestDb(t)

	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RememberVersion("wlx_Demo", "Demo", "2.0"); err != nil {
		t.Fatal(err)
	}
	// Empty inputs must be no-ops, not junk records.
	if err := db.RememberVersion("", "n", "9"); err != nil || db.RememberVersion("x", "n", "") != nil {
		t.Fatalf("empty RememberVersion inputs should be ignored: %v %v", err, err)
	}
	if _, exists := db.records["x"]; exists {
		t.Error("empty version must not create a record")
	}

	reloaded, err := NewDatabase("")
	if err != nil {
		t.Fatalf("records file invalid after write: %v", err)
	}

	plugins := reloaded.ApplyOverrides([]domain.Plugin{
		{ID: "wlx_Demo", Name: "Demo", InstalledVersion: "Unknown", Confidence: domain.ConfidenceFallback},
	})
	got := plugins[0]
	if got.InstalledVersion != "2.0" || got.Confidence != domain.ConfidenceManual {
		t.Fatalf("remembered version not applied: %+v", got)
	}
}

func TestMigrateLegacyIDs(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.SetUserOverride("WLX_CudaLister", domain.Override{CustomSource: "github:Alexey-T/CudaLister"}); err != nil {
		t.Fatal(err)
	}
	if err := db.RememberVersion("WCX_Total7zip", "Total7zip", "3.16"); err != nil {
		t.Fatal(err)
	}

	moved, err := db.MigrateLegacyIDs(map[string]string{
		"WLX_CudaLister":  "WLX:plugins/wlx/cudalister/cuda.wlx",
		"WCX_Total7zip":   "WCX:plugins/wcx/total7zip/total7zip64.wcx",
		"WCX_Nonexistent": "WCX:plugins/nope.nope",
	})
	if err != nil {
		t.Fatalf("MigrateLegacyIDs: %v", err)
	}
	if moved != 2 {
		t.Errorf("moved = %d, want 2", moved)
	}

	plugins := []domain.Plugin{
		{ID: "WLX:plugins/wlx/cudalister/cuda.wlx", Name: "Cuda"},
		{ID: "WCX:plugins/wcx/total7zip/total7zip64.wcx", Name: "7zip"},
	}
	res := db.ApplyOverrides(plugins)
	if res[0].CustomSource != "github:Alexey-T/CudaLister" {
		t.Errorf("override not migrated: %+v", res[0])
	}
	if res[1].InstalledVersion != "3.16" || res[1].Confidence != domain.ConfidenceManual {
		t.Errorf("record not migrated: %+v", res[1])
	}

	// Reload from disk: legacy keys must be gone, new keys persisted.
	reloaded, err := NewDatabase(db.filePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.overrides["WLX_CudaLister"]; ok {
		t.Error("legacy override key still present on disk")
	}
	if _, ok := reloaded.records["WCX_Total7zip"]; ok {
		t.Error("legacy record key still present on disk")
	}
	if _, ok := reloaded.overrides["WLX:plugins/wlx/cudalister/cuda.wlx"]; !ok {
		t.Error("migrated override not persisted")
	}
	if rec, ok := reloaded.records["WCX:plugins/wcx/total7zip/total7zip64.wcx"]; !ok || rec.Version != "3.16" {
		t.Errorf("migrated record not persisted: %+v", rec)
	}
}

func TestMigrateLegacyIDsNoopWhenTargetExists(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.SetUserOverride("WLX_Old", domain.Override{CustomVersion: "1.0"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserOverride("WLX:newid", domain.Override{CustomVersion: "2.0"}); err != nil {
		t.Fatal(err)
	}

	moved, err := db.MigrateLegacyIDs(map[string]string{"WLX_Old": "WLX:newid"})
	if err != nil {
		t.Fatal(err)
	}
	if moved != 0 {
		t.Errorf("moved = %d, want 0 (target key must win)", moved)
	}
	if db.overrides["WLX:newid"].CustomVersion != "2.0" {
		t.Error("existing target override was overwritten")
	}
	if db.overrides["WLX_Old"].CustomVersion != "1.0" {
		t.Error("legacy entry should stay untouched when target exists")
	}
}

func TestRememberVersionAppendsHistoryNewestFirst(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.RememberVersion("WCX:a", "Alpha", "1.0"); err != nil {
		t.Fatal(err)
	}
	if err := db.RememberVersion("WLX:b", "Beta", "2.0"); err != nil {
		t.Fatal(err)
	}

	hist := db.UpdateHistory()
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	if hist[0].PluginID != "WLX:b" || hist[0].Version != "2.0" || hist[0].PluginName != "Beta" {
		t.Errorf("newest entry = %+v", hist[0])
	}
	if hist[1].PluginID != "WCX:a" {
		t.Errorf("older entry = %+v", hist[1])
	}

	reloaded, err := NewDatabase(db.filePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.UpdateHistory(); len(got) != 2 || got[0].PluginID != "WLX:b" {
		t.Errorf("history not persisted: %+v", got)
	}
}

func TestRememberVersionClearsStaleManualVersion(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.SetUserOverride("WLX:c", domain.Override{
		CustomVersion: "1.8",
		CustomSource:  "github:Alexey-T/CudaLister",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserOverride("WLX:d", domain.Override{CustomVersion: "0.9"}); err != nil {
		t.Fatal(err)
	}

	if err := db.RememberVersion("WLX:c", "CudaLister", "1.8.6.3"); err != nil {
		t.Fatal(err)
	}
	if err := db.RememberVersion("WLX:d", "Delta", "1.0"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	plugins := reloaded.ApplyOverrides([]domain.Plugin{
		{ID: "WLX:c", InstalledVersion: "1.8.5"},
		{ID: "WLX:d", InstalledVersion: "0.9"},
	})
	if plugins[0].InstalledVersion != "1.8.6.3" {
		t.Errorf("updated version must win over stale manual version, got %q", plugins[0].InstalledVersion)
	}
	if plugins[0].CustomSource != "github:Alexey-T/CudaLister" {
		t.Errorf("custom source must survive, got %q", plugins[0].CustomSource)
	}
	if plugins[1].InstalledVersion != "1.0" {
		t.Errorf("record version must win once override is dropped, got %q", plugins[1].InstalledVersion)
	}
	if _, ok := reloaded.overrides["WLX:d"]; ok {
		t.Error("override with only a custom version must be removed after update")
	}
}

func TestHistoryIsCapped(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	db.history = make([]domain.UpdateEntry, MaxHistoryEntries)
	if err := db.RememberVersion("WCX:new", "New", "9.9"); err != nil {
		t.Fatal(err)
	}
	hist := db.UpdateHistory()
	if len(hist) != MaxHistoryEntries {
		t.Fatalf("history len = %d, want cap %d", len(hist), MaxHistoryEntries)
	}
	if hist[0].PluginID != "WCX:new" {
		t.Error("new entry must be at the head after capping")
	}
}
