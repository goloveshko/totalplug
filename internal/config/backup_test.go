package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSettingsFile(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(DataDirEnv, dir)
	if err := os.WriteFile(filepath.Join(dir, SettingsFileName), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBackupPolicyWithoutAFileUsesDefaults(t *testing.T) {
	t.Setenv(DataDirEnv, t.TempDir())

	if got := LoadBackupPolicy(); got != DefaultBackupPolicy() {
		t.Errorf("LoadBackupPolicy() = %+v, want %+v", got, DefaultBackupPolicy())
	}
}

// A settings file written before these keys existed says nothing about retention,
// so it must not read as "the user asked for no floor and no age limit".
func TestLoadBackupPolicyWithoutTheSectionUsesDefaults(t *testing.T) {
	writeSettingsFile(t, `{"window":{"x":1,"y":2,"width":800,"height":600},"tc_ini":"C:\\tc\\wincmd.ini"}`)

	if got := LoadBackupPolicy(); got != DefaultBackupPolicy() {
		t.Errorf("LoadBackupPolicy() = %+v, want %+v", got, DefaultBackupPolicy())
	}
}

func TestLoadBackupPolicyReadsTheSection(t *testing.T) {
	writeSettingsFile(t, `{"backup":{"keep_newest":5,"max_age_days":90}}`)

	want := BackupPolicy{KeepNewest: 5, MaxAgeDays: 90}
	if got := LoadBackupPolicy(); got != want {
		t.Errorf("LoadBackupPolicy() = %+v, want %+v", got, want)
	}
}

// Zero days is a choice, not an absent key: it means nothing is ever deleted.
func TestLoadBackupPolicyKeepsZeroDaysAsAChoice(t *testing.T) {
	writeSettingsFile(t, `{"backup":{"keep_newest":3,"max_age_days":0}}`)

	got := LoadBackupPolicy()
	if got.MaxAgeDays != 0 {
		t.Errorf("MaxAgeDays = %d, want the stored 0", got.MaxAgeDays)
	}
	if _, limited := got.AgeLimit(); limited {
		t.Error("AgeLimit() reported a limit for zero days")
	}
}

func TestLoadBackupPolicyClampsNegatives(t *testing.T) {
	writeSettingsFile(t, `{"backup":{"keep_newest":-4,"max_age_days":-1}}`)

	want := BackupPolicy{KeepNewest: 0, MaxAgeDays: 0}
	if got := LoadBackupPolicy(); got != want {
		t.Errorf("LoadBackupPolicy() = %+v, want %+v", got, want)
	}
}

func TestLoadBackupPolicyOnCorruptFileUsesDefaults(t *testing.T) {
	writeSettingsFile(t, `{"backup": `)

	if got := LoadBackupPolicy(); got != DefaultBackupPolicy() {
		t.Errorf("LoadBackupPolicy() = %+v, want %+v", got, DefaultBackupPolicy())
	}
}

func TestBackupSpacePathIsInsideTheDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(DataDirEnv, dir)

	if got, want := BackupSpacePath(), filepath.Join(dir, "backups"); got != want {
		t.Errorf("BackupSpacePath() = %q, want %q", got, want)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("naming the path must not create anything, dir = %v err = %v", entries, err)
	}

	created, err := BackupSpace()
	if err != nil {
		t.Fatalf("BackupSpace: %v", err)
	}
	if info, err := os.Stat(created); err != nil || !info.IsDir() {
		t.Errorf("BackupSpace() should create %s: %v", created, err)
	}
}
