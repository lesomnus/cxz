package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/cxz/internal/workspace"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return
		}
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
	defaultRoot := filepath.Join(base, "cxz")
	if v := os.Getenv("CXZ_STATE"); v != "" {
		defaultRoot = v
	}
	root := f.String("state", defaultRoot, "private persistent state directory")
	agent := f.String("agent", "claude", "Claude executable (serve only)")
	cfg := f.String("claude-config", os.Getenv("CLAUDE_CONFIG_DIR"), "existing Claude config directory (serve only)")
	f.Usage = func() {
		fmt.Fprintln(f.Output(), `cxz [--state DIR] COMMAND
  install [--workspace-root PATH] [--recreate]  Install background Docker manager
  uninstall                                  Remove manager; retain project data
  up|new [PATH] [--agent claude|codex] [--config FILE] [--no-attach]
  recreate [PATH|PROJECT] --yes               Replace container; keep workspace/volumes
  down [PATH|PROJECT]                        Remove owned project containers
  attach|it [SESSION|PROJECT]                 Attach TUI (Ctrl-C detaches)
  tui|watch                                  Multi-project TUI
  projects | ls | get SESSION                 Inspect projects/sessions
  login PROJECT --agent claude|codex          Project-local vendor login
  shell PROJECT | exec PROJECT -- COMMAND    Enter owned project environment
  send ID TEXT | reply ID REQUEST allow|deny [ANSWERS_JSON]
  interrupt ID | resume ID | stop ID | events ID [AFTER_SEQ]
  serve                                      Foreground local development server
Global flags precede COMMAND. Stop terminates the agent; down also removes containers.
Foreign devcontainers are never adopted. Inspect before recreate or --trust-config.`)
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
	case "_ready":
		conn, e := server.Dial(*root)
		if e != nil {
			return e
		}
		defer conn.Close()
		q, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, e = api.NewSessionsClient(conn).List(q, &api.Empty{})
		return e
	case "_boot":
		return workspace.Boot(*root)
	case "_project":
		runtime, e := workspace.LoadRuntime(*root)
		if e != nil {
			return e
		}
		os.Setenv("CXZ_PROJECT_ID", runtime.ProjectID)
		os.Setenv("CXZ_STATE", *root)
		return server.Run(ctx, *root, runtime.Claude, "")
	case "_login":
		if len(args) != 2 {
			return fmt.Errorf("agent required")
		}
		return workspace.Login(*root, args[1])
	case "install":
		flags := flag.NewFlagSet("install", flag.ContinueOnError)
		image := flags.String("image", "", "manager image (default builds from this binary)")
		workspace := flags.String("workspace-root", "", "engine-visible workspace root")
		recreate := flags.Bool("recreate", false, "replace owned manager, retaining state")
		if e = flags.Parse(args[1:]); e != nil {
			return e
		}
		return installer.Install(ctx, *root, *workspace, *image, *recreate, os.Stderr)
	case "uninstall":
		return installer.Uninstall(ctx, *root)
	case "_bridge":
		return transport.Bridge(*root)
	case "serve":
		if os.Getenv("CXZ_OWNER") != "" {
			return server.Run(ctx, *root, *agent, *cfg)
		}
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
	case "_guard":
		if len(args) != 2 {
			return fmt.Errorf("process group required")
		}
		return supervisor.Guard(args[1])
	}
	if _, err := transport.Load(*root); os.IsNotExist(err) {
		if os.Getenv("CXZ_PROJECT_ID") == "" {
			if _, err := os.Stat(server.Socket(*root)); os.IsNotExist(err) {
				return fmt.Errorf("cxz is not installed; run cxz install first")
			}
		}
	} else if err != nil {
		return err
	}
	conn, e := server.Dial(*root)
	if e != nil {
		return e
	}
	defer conn.Close()
	client := api.NewSessionsClient(conn)
	if args[0] == "login" || args[0] == "shell" || args[0] == "exec" {
		return projectExec(ctx, client, args[0], args[1:])
	}
	if args[0] == "up" || args[0] == "new" || args[0] == "recreate" || args[0] == "down" {
		return projectCommand(ctx, client, args[0], args[1:])
	}
	if args[0] == "attach" || args[0] == "it" {
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		return attach(ctx, client, target)
	}
	if args[0] == "tui" || args[0] == "watch" {
		return tui.Run(ctx, client)
	}
	callCtx, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	var result any
	switch args[0] {
	case "projects":
		result, e = client.Projects(callCtx, &api.Empty{})
	case "_new-local":
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
