package gui

// The window backdrop has to be known before wails.Run creates the HWND, so it
// is persisted in settings.json — which is read at that moment — instead of the
// frontend localStorage. Changing it in Settings only takes effect after a
// restart.
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

// normalizeWindowEffect maps an absent or unknown value onto the default, so a
// settings file written before the backdrop existed still answers with a value
// the window can use.
func normalizeWindowEffect(effect string) string {
	if !validWindowEffect(effect) {
		return defaultEffect
	}
	return effect
}
