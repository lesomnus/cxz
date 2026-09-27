// Package versionpin is the stable on-disk contract shared by release switching
// and automatic updates. Keep this format readable by the rollback release.
package versionpin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var tag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$`)

func Validate(version string) error {
	if !tag.MatchString(version) {
		return fmt.Errorf("use a published release tag such as v0.1.0")
	}
	return nil
}

type Pin struct {
	Ready           bool      `json:"ready"`
	Version         string    `json:"version"`
	Revision        string    `json:"revision"`
	Image           string    `json:"image,omitempty"`
	Generation      string    `json:"generation"`
	At              time.Time `json:"at"`
	PreviousEnabled *bool     `json:"previous_enabled,omitempty"`
}

func Load(root string) (Pin, error) {
	var p Pin
	b, e := os.ReadFile(filepath.Join(root, "version-pin.json"))
	if os.IsNotExist(e) {
		return p, nil
	}
	if e == nil {
		e = json.Unmarshal(b, &p)
	}
	return p, e
}
func Save(root string, p Pin) error {
	if e := Validate(p.Version); e != nil {
		return e
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	old, e := Load(root)
	if e != nil {
		return e
	}
	p.PreviousEnabled = old.PreviousEnabled
	if old.Version == "" {
		var policy struct {
			Enabled *bool `json:"enabled"`
		}
		b, e := os.ReadFile(filepath.Join(root, "cxz-update-policy.json"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil {
			if e = json.Unmarshal(b, &policy); e != nil {
				return e
			}
		}
		p.PreviousEnabled = policy.Enabled
	}
	if e = core.WriteJSON(filepath.Join(root, "version-pin.json"), p); e != nil {
		return e
	}
	// Also understood by the first automatic-update implementation.
	return core.WriteJSON(filepath.Join(root, "cxz-update-policy.json"), map[string]bool{"enabled": false})
}
func Clear(root string) error {
	p, e := Load(root)
	if e != nil || p.Version == "" {
		return e
	}
	if e = core.WriteJSON(filepath.Join(root, "cxz-update-policy.json"), struct {
		Enabled *bool `json:"enabled,omitempty"`
	}{p.PreviousEnabled}); e != nil {
		return e
	}
	return os.Remove(filepath.Join(root, "version-pin.json"))
}
func Check(root string) error {
	p, e := Load(root)
	if e != nil {
		return e
	}
	if p.Version != "" {
		return fmt.Errorf("cxz is pinned to %s; use cxz use VERSION to switch, or cxz use --unpin first", p.Version)
	}
	return nil
}

type clientKey struct{}
type Client struct{ Root, Executable string }

func WithClient(ctx context.Context, root string) context.Context {
	exe, e := os.Executable()
	if e != nil {
		return ctx
	}
	return context.WithValue(ctx, clientKey{}, Client{Root: root, Executable: exe})
}
func ClientFrom(ctx context.Context) (Client, bool) {
	c, ok := ctx.Value(clientKey{}).(Client)
	return c, ok
}

type Restart struct{ Executable string }

func (r *Restart) Error() string { return "frontend release changed; restarting" }
