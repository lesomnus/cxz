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
		reply, err := fetchRenderedDevcontainer(ctx, c)
		if err != nil {
			return err
		}
		dir, err := writeRenderedDevcontainer(flg.MustGet[string](c, "out"), reply)
		if err != nil {
			return err
		}
		return printRenderedDevcontainer(c, dir, reply)
	})
	// The merged Compose file is the one piece people read on its own -- a
	// volume's final name, an overridden image -- so it is available on stdout
	// to pipe, rather than only as a file in a directory. It sits beside render
	// rather than under it: a command cannot take an optional argument and have
	// subcommands, and "render docker-compose PROJECT" would be ambiguous about
	// which of the two the word is anyway.
	compose := &xli.Command{
		Name:  "docker-compose",
		Brief: "Print the merged Compose configuration in effect, as Compose resolves it",
		Synop: "The merge of the project's Compose files, the installation's shared override\nand cxz's own, in that order. This is the same file cxz devcontainer render\nwrites as compose/resolved.yaml.",
		Args:  arg.Args{mcpStringArg("PROJECT", true)},
	}
	compose.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		reply, err := fetchRenderedDevcontainer(ctx, c)
		if err != nil {
			return err
		}
		for _, f := range reply.Files {
			if f.Name == devcontainerrender.ResolvedCompose {
				_, err = c.Writer.Write(f.Data)
				return err
			}
		}
		if reply.Note != "" {
			return fmt.Errorf("no merged Compose configuration: %s", reply.Note)
		}
		return fmt.Errorf("no merged Compose configuration for %s", reply.Workspace)
	})
	return &xli.Command{
		Name:     "devcontainer",
		Brief:    "Inspect the devcontainer configuration cxz applies to a project",
		Commands: xli.Commands{render, compose},
		Handler:  xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) }),
	}
}

// fetchRenderedDevcontainer asks the manager what the project is running under.
// The project argument is optional: without one the current directory is used,
// and the manager walks up from it to the workspace that owns it.
func fetchRenderedDevcontainer(ctx context.Context, c *xli.Command) (*api.RenderDevcontainerReply, error) {
	// An optional argument may be absent, and MustGet panics on absent rather
	// than falling back to a zero value.
	target, _ := arg.Get[string](c, "PROJECT")
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		target = cwd
	}
	// A path has to be translated to what the engine sees; a project id or name
	// is already the manager's own handle for it.
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		var err error
		if target, err = dockerx.EnginePath(target); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	session := ""
	client, closeClient, err := configConnect(ctx, c, &target, &session)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	return client.RenderDevcontainer(ctx, &api.RenderDevcontainerInput{Handle: target})
}

// writeRenderedDevcontainer lays the files out as a directory you can open,
// which is the point: a merge order is easier to believe when the files sit in
// it. sources.txt keeps the explanation next to them, for when the terminal
// that printed it is gone.
func writeRenderedDevcontainer(out string, reply *api.RenderDevcontainerReply) (string, error) {
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

// renderedFile and renderedReply shape --format json output. They live here
// rather than being the reply itself: the keys are this command's interface,
// and data is deliberately absent because the bytes are the files just written.
type renderedFile struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Role   string `json:"role"`
	Data   []byte `json:"data"`
}
type renderedReply struct {
	Project   string         `json:"project"`
	Name      string         `json:"name"`
	Workspace string         `json:"workspace"`
	Note      string         `json:"note,omitempty"`
	Files     []renderedFile `json:"files"`
	Directory string         `json:"directory"`
}

func renderedView(dir string, reply *api.RenderDevcontainerReply) renderedReply {
	out := renderedReply{
		Project: reply.Project, Name: reply.Name, Workspace: reply.Workspace,
		Note: reply.Note, Directory: dir, Files: []renderedFile{},
	}
	for _, f := range reply.Files {
		out.Files = append(out.Files, renderedFile{Name: f.Name, Source: f.Source, Role: f.Role})
	}
	return out
}

func printRenderedDevcontainer(c *xli.Command, dir string, reply *api.RenderDevcontainerReply) error {
	if inheritedFlag(c, "format") == "json" {
		b, err := json.MarshalIndent(renderedView(dir, reply), "", "  ")
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
