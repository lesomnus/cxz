// Package settings holds non-secret client preferences. Vendor credentials and
// permission policy deliberately cannot be configured through this file.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Config struct {
	Agent       string `json:"agent,omitempty"`
	ClaudeModel string `json:"claude_model,omitempty"`
	CodexModel  string `json:"codex_model,omitempty"`
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
	d := json.NewDecoder(strings.NewReader(string(b)))
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
	return core.WriteJSON(filepath.Join(root, "settings.json"), c)
}
