//go:build !unix

package tui

import (
	"io"
	"os"
)

// Ctrl+S remains available when a terminal cannot distinguish modified Enter.
func keyboardInput(file *os.File) io.Reader { return file }
