package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The window backdrop must be known before wails.Run creates the HWND, so the
// choice lives in a plain-text file instead of the frontend localStorage.
// Changing it in Settings only takes effect after a restart.
const (
	effectNone    = "none"
	effectMica    = "mica"
	effectAcrylic = "acrylic"
	defaultEffect = effectMica
)

func validWindowEffect(effect string) bool {
	switch effect {
	case effectNone, effectMica, effectAcrylic:
		return true
	}
	return false
}

func windowEffectPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "TotalPlug", "window_effect.txt"), nil
}

// loadWindowEffect returns the configured effect, falling back to the default
// when the file is missing or holds an unknown value.
func loadWindowEffect() string {
	path, err := windowEffectPath()
	if err != nil {
		return effectNone
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultEffect
	}
	effect := strings.TrimSpace(string(data))
	if !validWindowEffect(effect) {
		return defaultEffect
	}
	return effect
}

func saveWindowEffect(effect string) error {
	if !validWindowEffect(effect) {
		return fmt.Errorf("unknown window effect %q", effect)
	}
	path, err := windowEffectPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(path, []byte(effect), 0644)
}
