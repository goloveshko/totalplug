package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

// newTestDb pins the data directory to a fresh temp folder through
// TOTALPLUG_DATA_DIR, so default paths stay hermetic and never touch the
// install this machine actually uses.
func newTestDb(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.DataDirEnv, dir)
	return dir
}

// remember logs a plain update — the shape most of these tests care about, where
// the copy the updater kept and the version it replaced are not the point.
func remember(t *testing.T, db *Database, pluginID, pluginName, version string) {
	t.Helper()
	if err := db.RecordChange(AppliedChange{
		PluginID:   pluginID,
		PluginName: pluginName,
		Version:    version,
	}); err != nil {
		t.Fatalf("RecordChange(%s %s): %v", pluginID, version, err)
	}
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

func TestRecordChangeRoundtripAppliesAsManual(t *testing.T) {
	newTestDb(t)

	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}
	remember(t, db, "wlx_Demo", "Demo", "2.0")
	// Empty inputs must be no-ops, not junk records.
	if err := db.RecordChange(AppliedChange{PluginID: "", PluginName: "n", Version: "9"}); err != nil {
		t.Fatalf("empty plugin id should be ignored: %v", err)
	}
	if err := db.RecordChange(AppliedChange{PluginID: "x", PluginName: "n", Version: ""}); err != nil {
		t.Fatalf("empty version should be ignored: %v", err)
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
	remember(t, db, "WCX_Total7zip", "Total7zip", "3.16")

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

func TestRecordChangeAppendsHistoryNewestFirst(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	remember(t, db, "WCX:a", "Alpha", "1.0")
	remember(t, db, "WLX:b", "Beta", "2.0")

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

// Both callers log the plugin they just updated the same way, so the mapping from
// a scanned plugin to a change is tested once, here.
func TestChangeFromUpdateMapsTheAppliedPlugin(t *testing.T) {
	got := ChangeFromUpdate(domain.Plugin{
		ID:               "WLX:c",
		Name:             "CudaLister",
		Path:             filepath.Join("C:", "TC", "wlx", "CudaLister", "cuda.wlx"),
		InstalledVersion: "1.8",
		LatestVersion:    "1.9",
	}, filepath.Join("C:", "data", "backups", "wlx", "CudaLister.bak_1700000000"))

	want := AppliedChange{
		PluginID:        "WLX:c",
		PluginName:      "CudaLister",
		Version:         "1.9",
		ReplacedVersion: "1.8",
		PluginDir:       filepath.Join("C:", "TC", "wlx", "CudaLister"),
		BackupPath:      filepath.Join("C:", "data", "backups", "wlx", "CudaLister.bak_1700000000"),
	}
	if got != want {
		t.Errorf("ChangeFromUpdate() = %+v, want %+v", got, want)
	}
}

// A rollback is a change like any other: it names the copy it put back and the
// version that copy held, so the line that was rolled back and the line that
// rolled it back describe the same two states from opposite ends.
func TestRecordChangeLogsRestoreWithItsSnapshot(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	update := AppliedChange{
		PluginID:        "WLX:c",
		PluginName:      "CudaLister",
		Version:         "1.9",
		ReplacedVersion: "1.8",
		PluginDir:       `C:\TC\wlx\CudaLister`,
		BackupPath:      `C:\data\backups\wlx\CudaLister.bak_1700000000`,
	}
	if err := db.RecordChange(update); err != nil {
		t.Fatal(err)
	}
	snapshot := `C:\data\backups\wlx\CudaLister.bak_1700000100`
	if err := db.RecordChange(ChangeFromRestore(db.UpdateHistory()[0], snapshot)); err != nil {
		t.Fatal(err)
	}

	hist := db.UpdateHistory()
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	restored := hist[0]
	if !restored.Restored || restored.Version != "1.8" || restored.ReplacedVersion != "1.9" {
		t.Errorf("restore line = %+v, want 1.8 replacing 1.9", restored)
	}
	if restored.PluginID != "WLX:c" || restored.PluginDir != update.PluginDir {
		t.Errorf("restore line lost the plugin it belongs to: %+v", restored)
	}
	if restored.BackupPath != snapshot {
		t.Errorf("restore line points at %q, want the snapshot of what it replaced", restored.BackupPath)
	}
	if got := db.records["WLX:c"].Version; got != "1.8" {
		t.Errorf("remembered version = %q, want the one now in the folder", got)
	}
}

// A plugin the scanner never read a version for has no version to report after a
// rollback either. The line still belongs in the log; the remembered record is
// left as it was, which is what lets the next scan fill it in.
func TestRecordChangeRestoreWithoutVersionKeepsTheRecord(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}

	remember(t, db, "WCX:u", "Uncertain", "2.0")
	if err := db.RecordChange(AppliedChange{
		PluginID:   "WCX:u",
		PluginName: "Uncertain",
		PluginDir:  `C:\TC\wcx\Uncertain`,
		BackupPath: `C:\data\backups\wcx\Uncertain.bak_1700000000`,
		Restored:   true,
	}); err != nil {
		t.Fatal(err)
	}

	if got := db.records["WCX:u"].Version; got != "2.0" {
		t.Errorf("remembered version = %q, want it left alone", got)
	}
	hist := db.UpdateHistory()
	if len(hist) != 2 || !hist[0].Restored || hist[0].Version != "" {
		t.Errorf("history = %+v, want the restore logged without a version", hist)
	}
}

func TestRecordChangeClearsStaleManualVersion(t *testing.T) {
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

	remember(t, db, "WLX:c", "CudaLister", "1.8.6.3")
	remember(t, db, "WLX:d", "Delta", "1.0")

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
	remember(t, db, "WCX:new", "New", "9.9")
	hist := db.UpdateHistory()
	if len(hist) != MaxHistoryEntries {
		t.Fatalf("history len = %d, want cap %d", len(hist), MaxHistoryEntries)
	}
	if hist[0].PluginID != "WCX:new" {
		t.Error("new entry must be at the head after capping")
	}
}

// TestInterruptedSaveKeepsThePreviousFile covers why the savers write a temp
// file and rename it: a save that truncates its target before encoding turns a
// crash into a lost database, because the next start quarantines the truncated
// file and saves the now-empty state over it.
func TestInterruptedSaveKeepsThePreviousFile(t *testing.T) {
	newTestDb(t)
	db, err := NewDatabase("")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserOverride("WLX:kept", domain.Override{CustomVersion: "1.2.3"}); err != nil {
		t.Fatal(err)
	}

	path := db.overridesPath
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A value json cannot encode fails the write after the temp file exists.
	if err := writeJSONAtomic(path, map[string]any{"boom": make(chan int)}); err == nil {
		t.Fatal("expected an unencodable value to fail the save")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("failed save changed the file:\nbefore: %s\nafter:  %s", before, after)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("failed save left a temp file behind: %v", err)
	}

	// A good save must answer for the same file, and leave no temp.
	if err := db.SetUserOverride("WLX:again", domain.Override{CustomVersion: "2.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("successful save left a temp file behind: %v", err)
	}
	reloaded, err := NewDatabase(db.filePath)
	if err != nil {
		t.Fatalf("the file a save wrote must load cleanly: %v", err)
	}
	if got := reloaded.overrides["WLX:kept"].CustomVersion; got != "1.2.3" {
		t.Errorf("override written before the failed save was lost, got %q", got)
	}
}
