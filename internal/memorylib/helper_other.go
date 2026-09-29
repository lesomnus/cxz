//go:build !linux

package memorylib

import "github.com/lesomnus/cxz/internal/core"

func helperIdentity(string, core.Session) error { return nil }
