package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goloveshko/totalplug/internal/config"
)

func TestSettingsRoundTrip(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	want := settings{
		Window: windowGeometry{X: 120, Y: 80, Width: 1280, Height: 800, Maximised: true},
	}
	if err := writeSettings(want); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}

	got, ok := loadSettings()
	if !ok {
		t.Fatal("settings written by writeSettings must load back")
	}
	if got != want {
		t.Fatalf("loadSettings() = %+v, want %+v", got, want)
	}
}

func TestTcIniChoiceSurvivesTheCloseSave(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	ini := filepath.Join(t.TempDir(), "wincmd.ini")
	if err := os.WriteFile(ini, []byte("[configuration]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	geometry := windowGeometry{X: 10, Y: 20, Width: 640, Height: 480}
	app.seedSettings(settings{Window: geometry})

	// A path the scanner could never open must be refused where the user typed
	// it, not later as a failed scan.
	if err := app.SetTcConfig(filepath.Join(filepath.Dir(ini), "gone.ini")); err == nil {
		t.Error("SetTcConfig accepted a file that does not exist")
	}
	if got := app.snapshotSettings().TcIni; got != "" {
		t.Errorf("a refused path must leave the setting alone, got %q", got)
	}

	if err := app.SetTcConfig(ini); err != nil {
		t.Fatalf("SetTcConfig: %v", err)
	}
	if got := app.snapshotSettings().TcIni; got != ini {
		t.Errorf("TcIni = %q, want the resolved %q", got, ini)
	}

	cfg := app.GetTcConfig()
	if cfg.Configured != ini || cfg.Effective != ini {
		t.Errorf("GetTcConfig() = %+v, want the manual path as both configured and effective", cfg)
	}
	if !cfg.Readable {
		t.Error("an existing wincmd.ini must report as readable")
	}

	// The close writes the whole mirror, so choosing an ini must not cost the
	// geometry, and vice versa.
	if err := writeSettings(app.snapshotSettings()); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	got, ok := loadSettings()
	if !ok {
		t.Fatal("settings did not load back after the close save")
	}
	if got.Window != geometry {
		t.Errorf("geometry lost when saving the ini: %+v", got.Window)
	}
	if got.TcIni != ini {
		t.Errorf("TcIni = %q, want %q", got.TcIni, ini)
	}

	// Blank input is the explicit way back to auto-detection, so that it clears
	// the setting rather than failing on an empty path.
	if err := app.SetTcConfig("   "); err != nil {
		t.Fatalf("SetTcConfig(blank): %v", err)
	}
	if got := app.snapshotSettings().TcIni; got != "" {
		t.Errorf("TcIni = %q after blanking, want auto-detection", got)
	}
	// Detection is machine-dependent, so the only safe claim is that the panel
	// hands the choice back to it.
	if cfg := app.GetTcConfig(); cfg.Configured != "" || cfg.Effective != cfg.Detected {
		t.Errorf("GetTcConfig() = %+v, want the effective path to be the detected one", cfg)
	}
}

func TestBackdropNeedsWindows11_22H2(t *testing.T) {
	if got := backdropForBuild(micaMinBuild - 1); got != backdropNone {
		t.Errorf("backdropForBuild(%d) = %q, want %q", micaMinBuild-1, got, backdropNone)
	}
	if got := backdropForBuild(micaMinBuild); got != backdropMica {
		t.Errorf("backdropForBuild(%d) = %q, want %q", micaMinBuild, got, backdropMica)
	}
	if got := backdropForBuild(26100); got != backdropMica {
		t.Errorf("backdropForBuild(26100) = %q, want %q", got, backdropMica)
	}
}

func TestLoadSettingsRejectsUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.DataDirEnv, dir)

	path := filepath.Join(dir, config.SettingsFileName)
	if err := os.WriteFile(path, []byte(`{"window": `), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadSettings(); ok {
		t.Error("corrupt settings must fall back to defaults, not load")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadSettings(); ok {
		t.Error("a missing settings file is a first launch, not a saved state")
	}
}
