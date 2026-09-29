package main

import (
	"os"
	"path/filepath"
	"testing"

	"totalplug/internal/config"
)

func TestSettingsRoundTrip(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	want := settings{
		Window:       windowGeometry{X: 120, Y: 80, Width: 1280, Height: 800, Maximised: true},
		WindowEffect: effectNone,
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

func TestWindowEffectSurvivesTheCloseSave(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	app := NewApp()
	geometry := windowGeometry{X: 10, Y: 20, Width: 640, Height: 480}
	app.seedSettings(settings{Window: geometry})

	if err := app.SetWindowEffect(effectNone); err != nil {
		t.Fatalf("SetWindowEffect: %v", err)
	}
	if got := app.GetWindowEffect(); got != effectNone {
		t.Fatalf("GetWindowEffect() = %q, want %q", got, effectNone)
	}

	// The close writes the whole mirror, so choosing a backdrop while the app
	// runs must not cost the geometry, and vice versa.
	if err := writeSettings(app.snapshotSettings()); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	got, ok := loadSettings()
	if !ok {
		t.Fatal("settings did not load back after the close save")
	}
	if got.Window != geometry {
		t.Errorf("geometry lost when saving the backdrop: %+v", got.Window)
	}
	if got.WindowEffect != effectNone {
		t.Errorf("WindowEffect = %q, want %q", got.WindowEffect, effectNone)
	}

	if err := app.SetWindowEffect("aurora"); err == nil {
		t.Error("an unknown backdrop must be refused")
	}
}

func TestNormalizeWindowEffectFallsBackToTheDefault(t *testing.T) {
	// A settings file written before the backdrop existed holds "", and a value
	// from an older or hand-edited file may be unknown; both must answer with a
	// backdrop the window can actually use.
	if got := normalizeWindowEffect(""); got != defaultEffect {
		t.Errorf(`normalizeWindowEffect("") = %q, want %q`, got, defaultEffect)
	}
	if got := normalizeWindowEffect(effectAcrylic); got != effectAcrylic {
		t.Errorf("normalizeWindowEffect(acrylic) = %q, want it kept", got)
	}
}

func TestLoadSettingsRejectsUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.DataDirEnv, dir)

	path := filepath.Join(dir, settingsFileName)
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
