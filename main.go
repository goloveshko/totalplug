package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/goloveshko/totalplug/internal/gui"
)

//go:embed all:frontend/dist
var assets embed.FS

// The entry point stays in this package because wails build compiles the project
// root and //go:embed cannot reach above it; the window itself is internal/gui.
func main() {
	if err := gui.Run(assets); err != nil {
		// The builtin println has an implementation-defined format and no stream a
		// GUI-subsystem binary writes to, so this is the one failure the window
		// cannot show and the terminal would not see either.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
