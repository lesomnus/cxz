package main

import (
	"os"
	"os/exec"
)

func restartPinnedFrontend(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if e := cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}
