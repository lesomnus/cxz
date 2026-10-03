package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/devcontainerrender"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func devcontainerCommand() *xli.Command {
	render := &xli.Command{
		Name:  "render",
		Brief: "Write the devcontainer configuration a project runs under to a directory",
		Synop: "cxz never writes into your .devcontainer. Your configuration is read, and\ncxz's keys and Compose override are layered onto a copy it keeps. This writes\nout that copy, every Compose file in merge order, and the merged result.\nThe project must have been started once: this reports what is in effect, not\na guess at what would be.",
		Args:  arg.Args{mcpStringArg("PROJECT", true)},
		Flags: flg.Flags{mcpStringFlag("out", "Directory to write into (default: a new temporary directory)", "")},
	}
	render.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		// An optional argument may be absent, and MustGet panics on absent
		// rather than falling back to a zero value.
		target, _ := arg.Get[string](c, "PROJECT")
		if target == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			target = cwd
		}
		// A path has to be translated to what the engine sees; a project id or
		// name is already the manager's own handle for it.
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			if target, err = dockerx.EnginePath(target); err != nil {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		session := ""
		client, closeClient, err := configConnect(ctx, c, &target, &session)
		if err != nil {
			return err
		}
		defer closeClient()
		spec, err := json.Marshal(devcontainerrender.Request{Project: target})
		if err != nil {
			return err
		}
		out, err := client.Docker(ctx, &api.DockerInput{Action: "devcontainer-render", Spec: spec})
		if err != nil {
			return err
		}
		var reply devcontainerrender.Reply
		if err = json.Unmarshal([]byte(out.Status), &reply); err != nil {
			return err
		}
		dir, err := writeRenderedDevcontainer(flg.MustGet[string](c, "out"), reply)
		if err != nil {
			return err
		}
		return printRenderedDevcontainer(c, dir, reply)
	})
	return &xli.Command{
		Name:     "devcontainer",
		Brief:    "Inspect the devcontainer configuration cxz applies to a project",
		Commands: xli.Commands{render},
		Handler:  xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) }),
	}
}

// writeRenderedDevcontainer lays the files out as a directory you can open,
// which is the point: a merge order is easier to believe when the files sit in
// it. sources.txt keeps the explanation next to them, for when the terminal
// that printed it is gone.
func writeRenderedDevcontainer(out string, reply devcontainerrender.Reply) (string, error) {
	if len(reply.Files) == 0 {
		return "", fmt.Errorf("manager returned no devcontainer files")
	}
	dir := out
	if dir == "" {
		temp, err := os.MkdirTemp("", "cxz-devcontainer-")
		if err != nil {
			return "", err
		}
		dir = temp
	} else if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	notes := fmt.Sprintf("project %s (%s)\nworkspace %s\n\n", reply.Name, reply.Project, reply.Workspace)
	for _, f := range reply.Files {
		name := filepath.FromSlash(f.Name)
		if !filepath.IsLocal(name) {
			return "", fmt.Errorf("manager returned an unsafe path: %s", f.Name)
		}
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(file, f.Data, 0600); err != nil {
			return "", err
		}
		notes += fmt.Sprintf("%s\n  %s\n  source: %s\n", f.Name, f.Role, f.Source)
	}
	if reply.Note != "" {
		notes += "\n" + reply.Note + "\n"
	}
	return dir, os.WriteFile(filepath.Join(dir, "sources.txt"), []byte(notes), 0600)
}

func printRenderedDevcontainer(c *xli.Command, dir string, reply devcontainerrender.Reply) error {
	if inheritedFlag(c, "format") == "json" {
		for i := range reply.Files {
			reply.Files[i].Data = nil // The bytes are the files just written.
		}
		b, err := json.MarshalIndent(struct {
			devcontainerrender.Reply
			Directory string `json:"directory"`
		}{reply, dir}, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(c.Writer, string(b))
		return err
	}
	fmt.Fprintln(c.Writer, dir)
	for _, f := range reply.Files {
		fmt.Fprintf(c.Writer, "  %-30s %s\n", f.Name, f.Role)
	}
	// Which project answered, since the directory the command ran in only has
	// to be somewhere inside its workspace.
	fmt.Fprintf(c.Writer, "\n%s · %s\n", reply.Name, reply.Workspace)
	if reply.Note != "" {
		fmt.Fprintln(c.Writer, "\n"+reply.Note)
	}
	return nil
}
