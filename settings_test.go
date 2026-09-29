package main

import (
	"os"
	"path/filepath"
	"testing"

	"totalplug/internal/config"
)

func TestSettingsRoundTrip(t *testing.T) {
	t.Setenv(config.DataDirEnv, t.TempDir())

	want := settings{Window: windowGeometry{X: 120, Y: 80, Width: 1280, Height: 800, Maximised: true}}
	writeSettings(want)

	got, ok := loadSettings()
	if !ok {
		t.Fatal("settings written by writeSettings must load back")
	}
	if got != want {
		t.Fatalf("loadSettings() = %+v, want %+v", got, want)
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
