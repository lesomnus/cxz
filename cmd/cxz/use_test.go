package main

import (
	"bytes"
	"context"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/versionpin"
	"strings"
	"testing"
)

func TestUseStatusUnpinAndInvalidRelease(t *testing.T) {
	state := t.TempDir()
	p := versionpin.Pin{Version: "v0.1.0", Generation: core.ID(), Ready: true}
	if e := versionpin.Save(state, p); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"use"}, {"use", "main"}, {"use", "--unpin"}} {
		var output bytes.Buffer
		c := newRoot(state)
		c.Writer = &output
		c.ErrWriter = &output
		c.ReadCloser = nil
		e := c.Run(context.Background(), args)
		if len(args) > 1 && args[1] == "main" {
			if e == nil {
				t.Fatal("moving tag accepted")
			}
			continue
		}
		if e != nil {
			t.Fatal(e)
		}
		if len(args) == 1 && !strings.Contains(output.String(), "v0.1.0") {
			t.Fatal(output.String())
		}
	}
	if p, e := versionpin.Load(state); e != nil || p.Version != "" {
		t.Fatal(p, e)
	}
}
