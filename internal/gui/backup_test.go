package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goloveshko/totalplug/internal/config"
)

// The retention lives in the same file the window geometry does because the CLI
// reads it there, so a panel edit has to reach the file, survive the close
// rewrite, and still leave the geometry alone.
func TestBackupPolicyRoundTrip(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	app := NewApp()
	geometry := windowGeometry{X: 10, Y: 20, Width: 640, Height: 480}
	app.seedSettings(settings{Window: geometry})

	// A file that says nothing about retention is not a request for no floor and
	// no age limit.
	if got := app.GetBackupPolicy(); got != config.DefaultBackupPolicy() {
		t.Fatalf("GetBackupPolicy() = %+v, want %+v", got, config.DefaultBackupPolicy())
	}

	want := config.BackupPolicy{KeepNewest: 1, MaxAgeDays: 0}
	if err := app.SetBackupPolicy(want); err != nil {
		t.Fatalf("SetBackupPolicy: %v", err)
	}
	if got := app.GetBackupPolicy(); got != want {
		t.Errorf("GetBackupPolicy() = %+v, want %+v", got, want)
	}
	if got := config.LoadBackupPolicy(); got != want {
		t.Errorf("the CLI reads %+v from the file, want %+v", got, want)
	}

	if err := writeSettings(app.snapshotSettings()); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	got, ok := loadSettings()
	if !ok {
		t.Fatal("settings did not load back after the close save")
	}
	if got.Window != geometry {
		t.Errorf("geometry lost when saving the retention policy: %+v", got.Window)
	}
	if got.backupPolicy() != want {
		t.Errorf("retention lost across the close save: %+v", got.backupPolicy())
	}
}

func TestSetBackupPolicyClampsNegatives(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	app := NewApp()
	if err := app.SetBackupPolicy(config.BackupPolicy{KeepNewest: -2, MaxAgeDays: -5}); err != nil {
		t.Fatalf("SetBackupPolicy: %v", err)
	}
	if got := app.GetBackupPolicy(); got != (config.BackupPolicy{KeepNewest: 0, MaxAgeDays: 0}) {
		t.Errorf("GetBackupPolicy() = %+v, want the negatives clamped to 0", got)
	}
}

// The panel names the folder it cannot yet have made: showing where copies go is
// not a reason to create an empty one.
func TestBackupLocationDoesNotCreateTheFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.DataDirEnv, dir)

	got := NewApp().BackupLocation()
	if want := filepath.Join(dir, "backups"); got != want {
		t.Errorf("BackupLocation() = %q, want %q", got, want)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("%s must not exist after only naming it: %v", got, err)
	}
}
