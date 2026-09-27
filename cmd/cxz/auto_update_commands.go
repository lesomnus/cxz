package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
	"os"
	"path/filepath"
)

func automaticUpdateCommands() xli.Commands {
	var commands xli.Commands
	for _, name := range []string{"status", "check", "enable", "disable"} {
		server := false
		commands = append(commands, &xli.Command{Name: name, Brief: map[string]string{"status": "Print cxz automatic update state", "check": "Check the selected release channel without restarting", "enable": "Enable cxz automatic updates", "disable": "Pause cxz automatic updates"}[name], Flags: flg.Flags{&flg.Switch{Name: "server", Brief: "Operate on the locally installed manager instead of this frontend", Default: &server}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			root := c
			for root.HasParent() {
				root = root.Parent()
			}
			state, e := filepath.Abs(flg.MustGet[string](root, "state"))
			if e != nil {
				return e
			}
			if flg.MustGet[bool](c, "server") {
				b, e := serverUpdateCommand(ctx, state, c.Name)
				if e != nil {
					return e
				}
				_, e = c.Writer.Write(b)
				return e
			}
			b, e := localUpdateCommand(ctx, state, c.Name)
			if e != nil {
				return e
			}
			_, e = c.Writer.Write(b)
			return e
		})})
	}
	return commands
}
func localUpdateCommand(ctx context.Context, root, action string) ([]byte, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	lock, e := core.Lock(filepath.Join(root, "self-update.lock"))
	if e != nil {
		return nil, e
	}
	defer lock.Close()
	switch action {
	case "enable", "disable":
		if e = cxzupdate.SetPolicy(root, action == "enable"); e != nil {
			return nil, e
		}
	case "check":
		if _, e = cxzupdate.Check(ctx, root, true); e != nil {
			return nil, e
		}
	case "status":
	default:
		return nil, fmt.Errorf("invalid update action")
	}
	state, e := cxzupdate.Load(root)
	if e != nil {
		return nil, e
	}
	policy, e := cxzupdate.Policy(root)
	if e != nil {
		return nil, e
	}
	channel, e := versionpin.Channel(root)
	if e != nil {
		return nil, e
	}
	b, e := json.MarshalIndent(map[string]any{"channel": channel, "enabled": policy.Active(), "running": cxzupdate.Current(), "update": state}, "", "  ")
	return append(b, '\n'), e
}
