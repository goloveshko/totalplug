package gui

import "golang.org/x/sys/windows"

// The Mica backdrop exists from Windows 11 22H2 (build 22621). Below that DWM
// ignores the attribute, and Wails' translucency fallback is a layered window
// that lets the desktop show through the webview — the washed-out look that got
// the first attempt reverted. Those systems keep the solid background.
const (
	micaMinBuild = 22621

	backdropMica = "mica"
	backdropNone = "none"
)

// backdrop is decided once, before wails.Run creates the window, and never
// changes: DWM applies it at creation, so there is nothing to toggle later.
var backdrop = backdropForBuild(windows.RtlGetVersion().BuildNumber)

// backdropForBuild keeps the version check separable from this machine.
func backdropForBuild(build uint32) string {
	if build >= micaMinBuild {
		return backdropMica
	}
	return backdropNone
}
