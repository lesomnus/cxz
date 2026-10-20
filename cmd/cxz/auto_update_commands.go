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
	for _, name := range []string{"status", "check", "enable", "disable", "apply"} {
		server := false
		force := false
		flags := flg.Flags{&flg.Switch{Name: "server", Brief: "Operate on the locally installed manager instead of this frontend", Default: &server}}
		if name == "apply" {
			// Waiting for the next tick, and then for every session to look
			// quiet, is what makes testing an update cost twenty minutes.
			flags = append(flags, &flg.Switch{Name: "force", Brief: "Replace the manager without waiting for sessions to look idle", Default: &force})
		}
		commands = append(commands, &xli.Command{Name: name, Brief: map[string]string{"status": "Print cxz automatic update state", "check": "Check the selected release channel without restarting", "enable": "Enable cxz automatic updates", "disable": "Pause cxz automatic updates", "apply": "Roll the installed manager out now instead of on the next tick"}[name], Flags: flags, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			root := c
			for root.HasParent() {
				root = root.Parent()
			}
			state, e := filepath.Abs(flg.MustGet[string](root, "state"))
			if e != nil {
				return e
			}
			action := c.Name
			if action == "apply" && flg.MustGet[bool](c, "force") {
				action = "apply-force"
			}
			if flg.MustGet[bool](c, "server") {
				b, e := serverUpdateCommand(ctx, state, action)
				if e != nil {
					return e
				}
				_, e = c.Writer.Write(b)
				return e
			}
			b, e := localUpdateCommand(ctx, state, action)
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
	case "apply", "apply-force":
		// There is nothing to roll out on a frontend: it carries no manager and
		// no transaction. Say which flag is missing rather than nothing.
		return nil, fmt.Errorf("apply operates on the installed manager; add --server")
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
	// Why an installation is not updating is the question this command is
	// asked, and the answer used to live in a container that had already been
	// removed. Newest last, as the file keeps them.
	attempts, e := cxzupdate.Attempts(root)
	if e != nil {
		return nil, e
	}
	if len(attempts) > 5 {
		attempts = attempts[len(attempts)-5:]
	}
	out := map[string]any{"channel": channel, "enabled": policy.Active(), "running": cxzupdate.Current(), "update": state, "attempts": attempts}
	for k, v := range rolloutDetail(ctx, root) {
		out[k] = v
	}
	b, e := json.MarshalIndent(out, "", "  ")
	return append(b, '\n'), e
}
