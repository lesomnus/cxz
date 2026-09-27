package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/releasechannel"
	"github.com/lesomnus/cxz/internal/versionpin"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

type usePlan struct {
	Requested string
	Release   *releasechannel.Release
	Pin       versionpin.Pin
}

func usePlanPath(root string) string { return filepath.Join(root, "use-plan.json") }
func loadUsePlan(root string) (usePlan, error) {
	var p usePlan
	b, e := os.ReadFile(usePlanPath(root))
	if e == nil {
		e = json.Unmarshal(b, &p)
	}
	return p, e
}
func resolveUsePlan(ctx context.Context, root, requested string) (usePlan, error) {
	p, e := loadUsePlan(root)
	if e == nil {
		if p.Requested != requested {
			return p, fmt.Errorf("unfinished switch; retry cxz use %s", p.Requested)
		}
		return p, nil
	}
	if !os.IsNotExist(e) {
		return p, e
	}
	p.Requested = requested
	if strings.HasPrefix(requested, "@") {
		channel := strings.TrimPrefix(requested, "@")
		r, e := releasechannel.Resolve(ctx, channel)
		if e != nil {
			return p, e
		}
		p.Release = &r
		p.Pin = versionpin.Pin{Channel: channel, Version: r.Version, Revision: r.Revision, Image: r.Image}
	} else {
		p.Pin.Version = requested
	}
	return p, nil
}
func useStatus(root string) (any, error) {
	p, e := versionpin.Load(root)
	if e != nil {
		return nil, e
	}
	channel, e := versionpin.Channel(root)
	if e != nil {
		return nil, e
	}
	selection := "@" + channel
	mode := "channel"
	if p.Pinned() {
		selection = p.Version
		mode = "pinned"
	}
	revision := buildRevision
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, s := range b.Settings {
			if s.Key == "vcs.revision" && revision == "" {
				revision = s.Value
			}
		}
	}
	return struct {
		versionpin.Pin
		Selection string            `json:"selection"`
		Mode      string            `json:"mode"`
		Pinned    bool              `json:"pinned"`
		Following string            `json:"following"`
		Running   map[string]string `json:"running"`
	}{p, selection, mode, p.Pinned(), channel, map[string]string{"version": version, "revision": revision}}, nil
}
