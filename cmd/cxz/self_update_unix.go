//go:build !windows

package main

import "github.com/lesomnus/xli"

func selfUpdateHandler() xli.Handler { return onRun(runSelfUpdate) }
