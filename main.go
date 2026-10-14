package main

import (
	"embed"

	"github.com/goloveshko/totalplug/internal/gui"
)

//go:embed all:frontend/dist
var assets embed.FS

// The entry point stays in this package because wails build compiles the project
// root and //go:embed cannot reach above it; the window itself is internal/gui.
func main() {
	if err := gui.Run(assets); err != nil {
		println("Error:", err.Error())
	}
}
