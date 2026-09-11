package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/tui"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		base = filepath.Join(home, ".local", "state")
	}
	f := flag.NewFlagSet("cxz", flag.ContinueOnError)
	root := f.String("state", filepath.Join(base, "cxz"), "private persistent state directory")
	agent := f.String("agent", "claude", "Claude executable (serve only)")
	cfg := f.String("claude-config", os.Getenv("CLAUDE_CONFIG_DIR"), "existing Claude config directory (serve only)")
	f.Usage = func() {
		fmt.Fprintln(f.Output(), "cxz [--state DIR] [--agent PATH] COMMAND\nCommands: serve, tui, new WORKSPACE [TITLE], ls, get ID, send ID TEXT, reply ID REQUEST allow|deny [ANSWERS_JSON], interrupt ID, resume ID, stop ID, events ID [AFTER_SEQ]\nGlobal flags must precede COMMAND. TUI/daemon exit detaches; stop explicitly terminates an agent.")
		f.PrintDefaults()
	}
	if e := f.Parse(os.Args[1:]); e != nil {
		return e
	}
	args := f.Args()
	if len(args) == 0 {
		f.Usage()
		return nil
	}
	var e error
	*root, e = filepath.Abs(*root)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch args[0] {
	case "serve":
		*agent, e = exec.LookPath(*agent)
		if e != nil {
			return e
		}
		*agent, e = filepath.Abs(*agent)
		if e != nil {
			return e
		}
		if *cfg != "" {
			*cfg, e = filepath.Abs(*cfg)
			if e != nil {
				return e
			}
		}
		return server.Run(ctx, *root, *agent, *cfg)
	case "_supervise":
		if len(args) != 2 {
			return fmt.Errorf("session id required")
		}
		return supervisor.Run(ctx, *root, args[1])
	}
	conn, e := server.Dial(*root)
	if e != nil {
		return e
	}
	defer conn.Close()
	client := api.NewSessionsClient(conn)
	if args[0] == "tui" {
		return tui.Run(ctx, client)
	}
	callCtx, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	var result any
	switch args[0] {
	case "new":
		if len(args) < 2 {
			return fmt.Errorf("workspace required")
		}
		path, e := filepath.Abs(args[1])
		if e != nil {
			return e
		}
		title := ""
		if len(args) > 2 {
			title = args[2]
		}
		result, e = client.Create(callCtx, &api.CreateRequest{Workspace: path, Title: title, ClientId: core.ID()})
	case "ls":
		result, e = client.List(callCtx, &api.Empty{})
	case "get", "send", "reply", "interrupt", "resume", "stop", "events":
		if len(args) < 2 {
			return fmt.Errorf("session id required")
		}
		id := args[1]
		if args[0] == "events" {
			var after uint64
			if len(args) > 2 {
				if _, e = fmt.Sscan(args[2], &after); e != nil {
					return e
				}
			}
			stream, e := client.Watch(ctx, &api.WatchRequest{SessionId: id, AfterSeq: after})
			if e != nil {
				return e
			}
			for {
				v, e := stream.Recv()
				if e != nil {
					if ctx.Err() != nil {
						return nil
					}
					return e
				}
				if e = json.NewEncoder(os.Stdout).Encode(v); e != nil {
					return e
				}
			}
		}
		var session *api.Session
		session, e = client.Get(callCtx, &api.SessionRef{Id: id})
		if e != nil {
			return e
		}
		control := &api.Control{SessionId: id, RunId: session.RunId, ClientId: core.ID()}
		switch args[0] {
		case "get":
			result = session
		case "send":
			if len(args) != 3 {
				return fmt.Errorf("send ID TEXT")
			}
			result, e = client.Send(callCtx, &api.Input{SessionId: id, RunId: session.RunId, ClientId: control.ClientId, Text: args[2]})
		case "reply":
			if len(args) < 4 || args[3] != "allow" && args[3] != "deny" {
				return fmt.Errorf("reply ID REQUEST allow|deny [ANSWERS_JSON]")
			}
			answers := ""
			if len(args) > 4 {
				answers = args[4]
			}
			result, e = client.Reply(callCtx, &api.Answer{SessionId: id, RunId: session.RunId, ClientId: control.ClientId, RequestId: args[2], Allow: args[3] == "allow", AnswersJson: answers})
		case "interrupt":
			result, e = client.Interrupt(callCtx, control)
		case "resume":
			result, e = client.Resume(callCtx, control)
		case "stop":
			result, e = client.Stop(callCtx, control)
		}
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
