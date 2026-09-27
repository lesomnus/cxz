//go:build !windows

package main

import (
	"os"
	"syscall"
)

func restartPinnedFrontend(path string) error { return syscall.Exec(path, os.Args, os.Environ()) }
