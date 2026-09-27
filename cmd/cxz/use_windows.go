package main

import (
	"context"
	"github.com/lesomnus/cxz/internal/versionpin"
	"io"
)

func prepareUseBackend(context.Context, string, versionpin.Pin, io.Writer) (*useBackend, error) {
	return nil, nil
}
func clearUseBackend(context.Context, string) error { return nil }
func useInternal([]string) (bool, error)            { return false, nil }
