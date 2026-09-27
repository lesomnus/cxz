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

func TestUsePlanResumesCapturedChannelWithoutResolvingLatest(t *testing.T) {
	root := t.TempDir()
	plan := usePlan{Requested: "@edge", Pin: versionpin.Pin{Channel: "edge", Version: "source-aaaaaaaaaaaa", Revision: strings.Repeat("a", 40), Generation: core.ID()}}
	if e := core.WriteJSON(usePlanPath(root), plan); e != nil {
		t.Fatal(e)
	}
	p, e := resolveUsePlan(context.Background(), root, "@edge")
	if e != nil || p.Pin.Generation != plan.Pin.Generation {
		t.Fatal(p, e)
	}
	if _, e = resolveUsePlan(context.Background(), root, "@stable"); e == nil {
		t.Fatal("unfinished snapshot replaced")
	}
	var output bytes.Buffer
	c := newRoot(root)
	c.Writer = &output
	c.ErrWriter = &output
	c.ReadCloser = nil
	if e = c.Run(context.Background(), []string{"use", "--unpin"}); e == nil {
		t.Fatal("unfinished snapshot cleared")
	}
}
func TestUseChannelStatusIsNotPinned(t *testing.T) {
	root := t.TempDir()
	if e := versionpin.Save(root, versionpin.Pin{Version: "v0.1.2", Channel: "stable", Ready: true}); e != nil {
		t.Fatal(e)
	}
	var output bytes.Buffer
	c := newRoot(root)
	c.Writer = &output
	c.ErrWriter = &output
	c.ReadCloser = nil
	if e := c.Run(context.Background(), []string{"use"}); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{`"selection":"@stable"`, `"pinned":false`, `"mode":"channel"`} {
		if !strings.Contains(output.String(), want) {
			t.Fatal(output.String())
		}
	}
}
