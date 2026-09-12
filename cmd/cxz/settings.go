package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"os"
	"path/filepath"
)

func settingsCommand() *xli.Command {
	c := &xli.Command{Name: "config", Brief: "Show non-secret client preferences", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		cfg, err := settings.Load(stateFrom(ctx))
		if err != nil {
			return err
		}
		return writeOutput(c, cfg)
	})}
	for _, op := range []string{"set", "unset"} {
		args := arg.Args{stringArg("KEY", false)}
		if op == "set" {
			args = append(args, stringArg("VALUE", false))
		}
		c.Commands = append(c.Commands, &xli.Command{Name: op, Brief: "Model defaults: claude-model, codex-model (unset agent cleans up the obsolete preference)", Args: args, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			root := stateFrom(ctx)
			if err := os.MkdirAll(root, 0700); err != nil {
				return err
			}
			lock, err := core.Lock(filepath.Join(root, "settings.lock"))
			if err != nil {
				return err
			}
			defer lock.Close()
			cfg, err := settings.Load(root)
			if err != nil {
				return err
			}
			value := ""
			if c.Name == "set" {
				value = arg.MustGet[string](c, "VALUE")
			}
			switch arg.MustGet[string](c, "KEY") {
			case "agent":
				cfg.Agent = value
			case "claude-model":
				cfg.ClaudeModel = value
			case "codex-model":
				cfg.CodexModel = value
			default:
				return fmt.Errorf("unknown preference; use agent, claude-model or codex-model")
			}
			if err = settings.Save(root, cfg); err != nil {
				return err
			}
			return writeOutput(c, cfg)
		})})
	}
	return c
}
