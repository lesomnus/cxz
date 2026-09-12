package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/supervisor"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"github.com/lesomnus/xli/tab"
)

func ptr[T any](v T) *T { return &v }

type stateKey struct{}

func stateFrom(ctx context.Context) string { return ctx.Value(stateKey{}).(string) }

type commandFunc func(context.Context, *xli.Command) error
type clientFunc func(context.Context, api.SessionsClient, *xli.Command) error

func onRun(fn commandFunc) xli.Handler {
	return xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		if err := validateInvocation(c); err != nil {
			return err
		}
		return fn(ctx, c)
	})
}

// Connection setup runs only for an executed API command, never help/completion.
func withClient(fn clientFunc) xli.Handler {
	return onRun(func(ctx context.Context, c *xli.Command) error {
		root := stateFrom(ctx)
		cfg, err := settings.Load(root)
		if err != nil {
			return err
		}
		ctx = settings.With(ctx, cfg)
		if _, err := transport.Load(root); os.IsNotExist(err) {
			if os.Getenv("CXZ_PROJECT_ID") == "" {
				if _, err := os.Stat(server.Socket(root)); os.IsNotExist(err) {
					return fmt.Errorf("cxz is not installed; run cxz manager install first")
				}
			}
		} else if err != nil {
			return err
		}
		conn, err := server.Dial(root)
		if err != nil {
			return err
		}
		defer conn.Close()
		return fn(ctx, resourceclient.New(conn), c)
	})
}

func stringArg(name string, optional bool) *arg.String {
	a := &arg.String{Name: name, Optional: optional}
	if optional {
		a.Default = ptr("")
	}
	return a
}
func stringFlag(name, brief, value string) *flg.String {
	return &flg.String{Name: name, Brief: brief, Default: ptr(value)}
}
func switchFlag(name, brief string) *flg.Switch {
	return &flg.Switch{Name: name, Brief: brief, Default: ptr(false)}
}

type agentParser struct{ flg.StringParser }

func (agentParser) Parse(v string) (string, error) {
	if v != "claude" && v != "codex" {
		return "", fmt.Errorf("agent must be claude or codex")
	}
	return v, nil
}

type decisionParser struct{ flg.StringParser }

func (decisionParser) Parse(v string) (string, error) {
	if v != "allow" && v != "deny" {
		return "", fmt.Errorf("decision must be allow or deny")
	}
	return v, nil
}

func agentFlag(value string) *flg.Base[string, agentParser] {
	f := &flg.Base[string, agentParser]{Name: "agent", Brief: "Agent: claude or codex (selected Account / existing session when omitted)", Default: ptr(value)}
	f.Handler = flg.OnTab[string](func(_ context.Context, t tab.Tab) error { t.Value("claude"); t.Value("codex"); return nil })
	return f
}

func agentArg() *arg.Mono[string, agentParser] {
	a := &arg.Mono[string, agentParser]{Name: "AGENT", Brief: "Required agent: claude or codex"}
	a.Handler = arg.OnTab[string](func(_ context.Context, t tab.Tab) { t.Value("claude"); t.Value("codex") })
	return a
}

func newRoot(state string) *xli.Command {
	root := &xli.Command{Name: "cxz", Brief: "Persistent coding-agent sessions in owned devcontainers",
		Synop: "Flags precede positional arguments: cxz account add codex work; cxz session new --account work .\nCtrl-C detaches the TUI; stop terminates the agent. Foreign containers are never adopted.",
		Flags: flg.Flags{stringFlag("state", "Private client/runtime state directory", state)},
		Handler: xli.Chain(xli.OnRunPass(func(ctx context.Context, c *xli.Command, next xli.Next) error {
			if err := validateInvocation(c); err != nil {
				return err
			}
			path, err := filepath.Abs(flg.MustGet[string](c, "state"))
			if err != nil {
				return err
			}
			return next(context.WithValue(ctx, stateKey{}, path))
		}), onRun(func(_ context.Context, c *xli.Command) error { return c.PrintHelp(c.Writer) })),
	}
	root.Commands = xli.Commands{
		{Name: "install", Brief: "Start background Docker manager", Flags: flg.Flags{stringFlag("workspace-root", "Engine-visible workspace root", ""), stringFlag("image", "Manager image (default: build from this binary)", ""), switchFlag("recreate", "Replace owned manager, preserving data")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			return installer.Install(ctx, stateFrom(ctx), flg.MustGet[string](c, "workspace-root"), flg.MustGet[string](c, "image"), flg.MustGet[bool](c, "recreate"), c.ErrWriter)
		})},
		{Name: "uninstall", Brief: "Remove manager; retain projects and volumes", Handler: onRun(func(ctx context.Context, _ *xli.Command) error { return installer.Uninstall(ctx, stateFrom(ctx)) })},
		{Name: "serve", Brief: "Run foreground development server", Flags: flg.Flags{stringFlag("agent", "Claude executable (not Account agent selection)", "claude")}, Handler: onRun(serveCommand)},
	}
	for _, name := range []string{"up", "new", "recreate", "down"} {
		root.Commands = append(root.Commands, newProjectCommand(name))
	}
	root.Commands = append(root.Commands, projectMetadataCommands())
	root.Commands = append(root.Commands, accountCommands())
	root.Commands = append(root.Commands, accountInternalCommands()...)
	root.Commands = append(root.Commands,
		&xli.Command{Name: "attach", Aliases: []string{"it"}, Brief: "Attach TUI to session/project", Args: arg.Args{projectArg("TARGET", true)}, Handler: withClient(func(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
			return attach(ctx, client, arg.MustGet[string](c, "TARGET"))
		})},
		&xli.Command{Name: "tui", Aliases: []string{"watch"}, Brief: "Open multi-project TUI", Handler: withClient(func(ctx context.Context, client api.SessionsClient, _ *xli.Command) error {
			return tui.Run(ctx, client)
		})},
	)
	for _, name := range []string{"shell", "exec"} {
		c := &xli.Command{Name: name, Brief: map[string]string{"shell": "Open project shell", "exec": "Execute command inside project"}[name], Args: arg.Args{projectArg("PROJECT", false), &arg.Remains{Name: "COMMAND", Optional: name == "shell"}}, Handler: withClient(projectExec)}
		root.Commands = append(root.Commands, c)
	}
	for _, name := range []string{"ls", "projects", "get", "send", "reply", "interrupt", "resume", "stop", "events", "_new-local"} {
		root.Commands = append(root.Commands, newSessionCommand(name))
	}
	root.Commands = append(root.Commands, internalCommands()...)
	root.Commands = append(root.Commands, settingsCommand(), doctorCommand(), logsCommand())
	root.Commands = append(root.Commands, releaseCommands()...)
	root.Commands = append(root.Commands, xli.NewCmdCompletion())
	reorganizeCommands(root)
	bindProjectCompletions(root)
	return root
}

func serveCommand(ctx context.Context, c *xli.Command) error {
	bin := flg.MustGet[string](c, "agent")
	if os.Getenv("CXZ_OWNER") == "" {
		var err error
		bin, err = exec.LookPath(bin)
		if err != nil {
			return err
		}
		bin, err = filepath.Abs(bin)
		if err != nil {
			return err
		}
	}
	return server.Run(ctx, stateFrom(ctx), bin, "")
}

func internalCommands() xli.Commands {
	makeCmd := func(name string, args arg.Args, fn commandFunc) *xli.Command {
		return &xli.Command{Name: name, Category: "Internal runtime", Brief: "Internal process entrypoint", Args: args, Handler: onRun(fn)}
	}
	return xli.Commands{
		makeCmd("_ready", nil, func(ctx context.Context, _ *xli.Command) error {
			conn, err := server.Dial(stateFrom(ctx))
			if err != nil {
				return err
			}
			defer conn.Close()
			q, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			_, err = resourceclient.New(conn).List(q, &api.Empty{})
			return err
		}),
		makeCmd("_boot", nil, func(ctx context.Context, _ *xli.Command) error { return workspace.Boot(stateFrom(ctx)) }),
		makeCmd("_bridge", nil, func(ctx context.Context, _ *xli.Command) error { return transport.Bridge(stateFrom(ctx)) }),
		makeCmd("_project", nil, func(ctx context.Context, _ *xli.Command) error {
			r, err := workspace.LoadRuntime(stateFrom(ctx))
			if err != nil {
				return err
			}
			os.Setenv("CXZ_PROJECT_ID", r.ProjectID)
			os.Setenv("CXZ_STATE", stateFrom(ctx))
			return server.Run(ctx, stateFrom(ctx), r.Claude, "")
		}),
		makeCmd("_supervise", arg.Args{stringArg("SESSION", false)}, func(ctx context.Context, c *xli.Command) error {
			return supervisor.Run(ctx, stateFrom(ctx), arg.MustGet[string](c, "SESSION"))
		}),
		makeCmd("_guard", arg.Args{stringArg("PID", false)}, func(_ context.Context, c *xli.Command) error { return supervisor.Guard(arg.MustGet[string](c, "PID")) }),
	}
}

func newSessionCommand(name string) *xli.Command {
	c := &xli.Command{Name: name, Brief: map[string]string{"ls": "List sessions as JSON", "projects": "List owned and foreign projects as JSON", "get": "Get session status", "send": "Send one message", "reply": "Answer a pending approval/question", "interrupt": "Interrupt active turn", "resume": "Explicitly resume session", "stop": "Terminate session agent", "events": "Stream journal events", "_new-local": "Internal local integration entrypoint"}[name], Handler: withClient(sessionCommand)}
	if name == "_new-local" {
		c.Category = "Internal runtime"
		c.Args = arg.Args{stringArg("ACCOUNT", false), stringArg("WORKSPACE", false), stringArg("TITLE", true)}
		return c
	}
	if name != "ls" && name != "projects" {
		c.Args = arg.Args{stringArg("SESSION", false)}
	}
	switch name {
	case "send":
		c.Args = append(c.Args, stringArg("TEXT", false))
	case "reply":
		decision := &arg.Mono[string, decisionParser]{Name: "DECISION"}
		decision.Handler = arg.OnTab[string](func(_ context.Context, t tab.Tab) { t.Value("allow"); t.Value("deny") })
		c.Args = append(c.Args, stringArg("REQUEST", false), decision, stringArg("ANSWERS_JSON", true))
	case "events":
		c.Args = append(c.Args, &arg.Uint64{Name: "AFTER_SEQ", Optional: true, Default: ptr(uint64(0))})
	}
	return c
}

func sessionCommand(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result any
	var err error
	switch c.Name {
	case "ls":
		if c.Parent().Name == "project" {
			result, err = client.Projects(call, &api.Empty{})
		} else {
			result, err = client.List(call, &api.Empty{})
		}
	case "_new-local":
		path, e := filepath.Abs(arg.MustGet[string](c, "WORKSPACE"))
		if e != nil {
			return e
		}
		result, err = client.Create(call, &api.CreateRequest{Workspace: path, Title: arg.MustGet[string](c, "TITLE"), ClientId: core.ID(), Account: arg.MustGet[string](c, "ACCOUNT")})
	case "events":
		stream, e := client.Watch(ctx, &api.WatchRequest{SessionId: arg.MustGet[string](c, "SESSION"), AfterSeq: arg.MustGet[uint64](c, "AFTER_SEQ")})
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
			if e = writeOutput(c, v); e != nil {
				return e
			}
		}
	default:
		id := arg.MustGet[string](c, "SESSION")
		s, e := client.Get(call, &api.SessionRef{Id: id})
		if e != nil {
			return e
		}
		control := &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()}
		switch c.Name {
		case "get":
			result = s
		case "send":
			result, err = client.Send(call, &api.Input{SessionId: id, RunId: s.RunId, ClientId: control.ClientId, Text: arg.MustGet[string](c, "TEXT")})
		case "reply":
			result, err = client.Reply(call, &api.Answer{SessionId: id, RunId: s.RunId, ClientId: control.ClientId, RequestId: arg.MustGet[string](c, "REQUEST"), Allow: arg.MustGet[string](c, "DECISION") == "allow", AnswersJson: arg.MustGet[string](c, "ANSWERS_JSON")})
		case "interrupt":
			result, err = client.Interrupt(call, control)
		case "resume":
			result, err = client.Resume(call, control)
		case "stop":
			result, err = client.Stop(call, control)
		}
	}
	if err != nil {
		return err
	}
	if c.Name == "_new-local" {
		return writeJSON(c.Writer, result)
	}
	return writeOutput(c, result)
}
