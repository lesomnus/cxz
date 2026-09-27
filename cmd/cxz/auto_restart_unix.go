//go:build !windows

package main

import "os"

func restoreFrontend(target, previous string) error { return os.Rename(previous, target) }
