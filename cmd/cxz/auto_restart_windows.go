package main

import (
	"github.com/lesomnus/cxz/internal/core"
	"os"
)

func restoreFrontend(target, previous string) error {
	failed := target + ".failed-" + core.ID()
	if e := os.Rename(target, failed); e != nil {
		return e
	}
	if e := os.Rename(previous, target); e != nil {
		_ = os.Rename(failed, target)
		return e
	}
	_ = os.Remove(failed)
	return nil
}
