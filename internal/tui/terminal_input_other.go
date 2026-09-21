//go:build !unix

package tui

import (
	"io"
	"os"
)

// Windows console records are handled by runKeyboardProgram. Other non-Unix
// inputs retain the default reader and the Ctrl+S fallback.
func keyboardInput(file *os.File) io.Reader { return file }

func recordedKeyboardInput(file *os.File, _ *debugRecorder) io.Reader { return file }

const extendedKeyboard = false
