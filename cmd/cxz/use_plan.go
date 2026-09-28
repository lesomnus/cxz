package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/releasechannel"
	"github.com/lesomnus/cxz/internal/selfupdate"
	"github.com/lesomnus/cxz/internal/versionpin"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// Cache the verified executable before persisting a transaction. A resumed
// transaction must retain its original release even after edge moves on.
func downloadUseChannel(ctx context.Context, root string, plan usePlan, out io.Writer) (selfupdate.Artifact, usePlan, error) {
	for attempt := 0; ; attempt++ {
		r := *plan.Release
		if err := r.Validate(); err != nil {
			return selfupdate.Artifact{}, plan, err
		}
		dir := filepath.Join(root, "use-downloads", r.Revision, runtime.GOOS+"-"+runtime.GOARCH, r.Assets[runtime.GOOS+"/"+runtime.GOARCH].SHA256)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return selfupdate.Artifact{}, plan, err
		}
		a, err := selfupdate.DownloadChannel(ctx, dir, runtime.GOOS, runtime.GOARCH, r, out)
		if err == nil || attempt != 0 || plan.Requested != "@edge" || plan.Pin.Generation != "" || !releasechannel.IsNotFound(err) {
			return a, plan, err
		}
		latest, err := releasechannel.Resolve(ctx, "edge")
		if err != nil {
			return a, plan, err
		}
		if latest.Revision == r.Revision || latest.Sequence < r.Sequence {
			return a, plan, fmt.Errorf("edge asset unavailable after manifest refresh")
		}
		plan.Release = &latest
		plan.Pin = versionpin.Pin{Channel: "edge", Version: latest.Version, Revision: latest.Revision, Image: latest.Image}
	}
}
