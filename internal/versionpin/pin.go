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

func ValidateSelection(value string) error {
	if value == "@edge" || value == "@stable" {
		return nil
	}
	return Validate(value)
}
func ValidateInstalled(value string) error {
	if regexp.MustCompile(`^source-[a-f0-9]{12}$`).MatchString(value) {
		return nil
	}
	return Validate(value)
}
func Channel(root string) (string, error) {
	var c struct {
		Channel string `json:"channel"`
	}
	b, e := os.ReadFile(filepath.Join(root, "update-channel.json"))
	if os.IsNotExist(e) {
		value := os.Getenv("CXZ_UPDATE_CHANNEL")
		if value == "" {
			value = "edge"
		}
		if value != "edge" && value != "stable" {
			return "", fmt.Errorf("invalid update channel")
		}
		return value, nil
	}
	if e != nil {
		return "", e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return "", e
	}
	if c.Channel != "edge" && c.Channel != "stable" {
		return "", fmt.Errorf("invalid update channel")
	}
	return c.Channel, nil
}
func (p Pin) Pinned() bool { return p.Version != "" && p.Channel == "" }
func (p Pin) Selection() string {
	if p.Channel != "" {
		return "@" + p.Channel
	}
	return p.Version
}

type Pin struct {
	Channel         string    `json:"channel,omitempty"`
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
	if e := ValidateInstalled(p.Version); e != nil {
		return e
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	if p.Channel != "" && p.Channel != "edge" && p.Channel != "stable" {
		return fmt.Errorf("invalid channel")
	}
	old, e := Load(root)
	if e != nil {
		return e
	}
	p.PreviousEnabled = old.PreviousEnabled
	if old.Version == "" || (old.Channel != "" && old.Generation != p.Generation) {
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
	if p.Ready && p.Channel != "" {
		if e = core.WriteJSON(filepath.Join(root, "update-channel.json"), map[string]string{"channel": p.Channel}); e != nil {
			return e
		}
	}
	if e = core.WriteJSON(filepath.Join(root, "version-pin.json"), p); e != nil {
		return e
	}
	// Also understood by the first automatic-update implementation.
	return core.WriteJSON(filepath.Join(root, "cxz-update-policy.json"), map[string]bool{"enabled": p.Ready && p.Channel != ""})
}
func Clear(root string) error {
	p, e := Load(root)
	if e != nil || !p.Pinned() {
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
	if p.Version != "" && !p.Ready {
		return fmt.Errorf("version switch unfinished; retry cxz use %s", p.Selection())
	}
	if p.Pinned() {
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
