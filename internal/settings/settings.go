// Package settings holds non-secret client preferences. Vendor credentials and
// permission policy deliberately cannot be configured through this file.
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/engine"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/tailscale/hujson"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Config struct {
	Connections *Connections      `json:"connections,omitempty"`
	Docker      engine.Config     `json:"docker,omitempty"`
	Files       []filemap.Mapping `json:"files,omitempty"`
	Agent       string            `json:"agent,omitempty"`
	ClaudeModel string            `json:"claude_model,omitempty"`
	CodexModel  string            `json:"codex_model,omitempty"`
}
type key struct{}

func With(ctx context.Context, c Config) context.Context { return context.WithValue(ctx, key{}, c) }
func From(ctx context.Context) Config                    { c, _ := ctx.Value(key{}).(Config); return c }
func (c Config) Model(agent string) string {
	if agent == "codex" {
		return c.CodexModel
	}
	return c.ClaudeModel
}
func ValidateModel(v string) error {
	if len(v) > 200 || strings.IndexFunc(v, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 || strings.HasPrefix(v, "-") {
		return fmt.Errorf("model must be a model ID or alias without whitespace (maximum 200 bytes)")
	}
	return nil
}
func (c Config) Validate() error {
	if c.Connections != nil {
		if err := c.Connections.Validate(); err != nil {
			return err
		}
	}
	if err := c.Docker.Validate(); err != nil {
		return err
	}
	for _, f := range c.Files {
		if f.Src == "" {
			return fmt.Errorf("file source required")
		}
		if err := filemap.ValidateMappingDestination(f.Dst, f.Agent); err != nil {
			return err
		}
	}
	if c.Agent != "" && c.Agent != "claude" && c.Agent != "codex" {
		return fmt.Errorf("agent must be claude or codex")
	}
	if err := ValidateModel(c.ClaudeModel); err != nil {
		return err
	}
	return ValidateModel(c.CodexModel)
}
func Load(root string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return Parse(b)
}

func Parse(b []byte) (Config, error) {
	standard, err := hujson.Standardize(bytes.Clone(b))
	if err != nil {
		return Config{}, fmt.Errorf("invalid settings.json: %w", err)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(standard), []byte("{")) {
		return Config{}, fmt.Errorf("settings.json must contain one JSON object")
	}
	var c Config
	d := json.NewDecoder(bytes.NewReader(standard))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid settings.json: %w", err)
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("settings.json must contain one JSON object")
	}
	return c, c.Validate()
}

func Save(root string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	path := filepath.Join(root, "settings.json")
	original, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		original = Template()
	} else if err != nil {
		return err
	}
	previous, err := Parse(original)
	if err != nil {
		return err
	}
	updated, err := updateDocument(original, previous, c)
	if err != nil {
		return err
	}
	return core.WriteFile(path, updated)
}
