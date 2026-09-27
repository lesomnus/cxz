package main

import (
	"os"
	"os/exec"
)

func restartPinnedFrontend(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Keep the invoking shell waiting while the replacement owns the console.
	return cmd.Run()
}
