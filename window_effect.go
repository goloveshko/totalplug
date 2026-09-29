package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"totalplug/internal/config"
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

// windowEffectPath lives in the data directory because it is read before the
// window exists, so the frontend's localStorage is not available yet.
func windowEffectPath() string {
	return filepath.Join(config.DataDir(), "window_effect.txt")
}

// loadWindowEffect returns the configured effect, falling back to the default
// when the file is missing or holds an unknown value.
func loadWindowEffect() string {
	data, err := os.ReadFile(windowEffectPath())
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
	return os.WriteFile(windowEffectPath(), []byte(effect), 0644)
}
