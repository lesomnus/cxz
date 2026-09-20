//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/payday/slug"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

// Called only in Run mode, before settings IO, connection setup or mutations.
// Checks requiring resource/vendor state remain in the relevant workflow.
func validateInvocation(c *xli.Command) error {
	for _, a := range c.Args {
		if a.IsMany() {
			continue
		}
		if value, set := arg.Get[string](c, a.Info().Name); set && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not be empty", a.Info().Name)
		}
	}
	for _, name := range []string{"state", "account", "agent", "model", "config", "name", "alias", "auth-backend", "image", "workspace-root", "project"} {
		if value, set := flg.Get[string](c, name); set && strings.TrimSpace(value) == "" {
			return fmt.Errorf("--%s must not be empty; omit it to use the default", name)
		}
	}
	if value, set := flg.Get[string](c, "account"); set {
		if err := accounts.Validate(value, "codex"); err != nil {
			return err
		}
	}
	if value, set := arg.Get[string](c, "ACCOUNT"); set {
		if err := accounts.Validate(value, "codex"); err != nil {
			return err
		}
	}
	if value, set := flg.Get[string](c, "model"); set {
		if err := settings.ValidateModel(value); err != nil {
			return err
		}
	}
	if value, set := flg.Get[string](c, "name"); set {
		if len(value) > 200 || strings.ContainsAny(value, "\r\n\t\x00") {
			return fmt.Errorf("display name must be at most 200 bytes without control whitespace")
		}
	}
	if value, set := flg.Get[string](c, "alias"); set {
		if _, err := slug.ParseAlias(value); err != nil {
			return err
		}
	}
	parent := ""
	if c.HasParent() {
		parent = c.Parent().Name
	}
	switch {
	case parent == "account" && c.Name == "add":
		_, err := accounts.Select(arg.MustGet[string](c, "AGENT"), flg.MustGet[string](c, "auth-backend"))
		return err
	case parent == "project" && c.Name == "set":
		_, name := flg.Get[string](c, "name")
		_, alias := flg.Get[string](c, "alias")
		if !name && !alias {
			return fmt.Errorf("project set requires at least one of --name or --alias")
		}
	case parent == "config" && (c.Name == "set" || c.Name == "unset"):
		key := arg.MustGet[string](c, "KEY")
		if key == "agent" && c.Name == "set" {
			return fmt.Errorf("agent is fixed by Account; use cxz account add AGENT ACCOUNT (config unset agent removes an old preference)")
		}
		if key != "agent" && key != "claude-model" && key != "codex-model" {
			return fmt.Errorf("unknown preference; use claude-model or codex-model (unset also accepts legacy agent)")
		}
		if c.Name == "set" {
			value := arg.MustGet[string](c, "VALUE")
			return settings.ValidateModel(value)
		}
	case c.Name == "new" && !terminal(c):
		if flg.MustGet[string](c, "account") == "" {
			return fmt.Errorf("new requires --account outside an interactive terminal; list profiles with cxz account ls")
		}
	case c.Name == "reply":
		if value, set := arg.Get[string](c, "ANSWERS_JSON"); set {
			var answers map[string]string
			if json.Unmarshal([]byte(value), &answers) != nil || answers == nil {
				return fmt.Errorf("ANSWERS_JSON must be a JSON object mapping question IDs to strings")
			}
		}
	case c.Name == "update":
		image := arg.MustGet[string](c, "IMAGE")
		last := image[strings.LastIndex(image, "/")+1:]
		if strings.ContainsAny(image, " \t\r\n") || (!strings.Contains(last, ":") && !strings.Contains(last, "@")) || strings.HasSuffix(last, ":") || strings.HasSuffix(last, "@") || strings.HasSuffix(image, ":latest") {
			return fmt.Errorf("update IMAGE requires an explicit version tag or digest (not latest)")
		}
	}
	return nil
}
