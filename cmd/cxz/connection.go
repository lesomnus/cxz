package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func connectionCommand() *xli.Command {
	group := &xli.Command{Name: "connection", Brief: "Inspect saved connections and open SSH port tunnels", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) })}
	group.Commands = xli.Commands{
		{Name: "ls", Brief: "List locally configured connections without connecting", Flags: flg.Flags{&flg.String{Name: "format", Brief: "Output format: table or json", Default: remoteDefault("table")}}, Handler: xli.OnRun(connectionList)},
		{Name: "tunnel", Brief: "Forward localhost to an SSH connection's remote loopback port; Ctrl+C closes it", Args: arg.Args{&arg.String{Name: "NAME"}}, Flags: flg.Flags{
			&flg.String{Name: "local-port", Brief: "Local IPv4 loopback port", Default: remoteDefault("7350")},
			&flg.String{Name: "remote-port", Brief: "Port on the SSH host's 127.0.0.1", Default: remoteDefault("7350")},
		}, Handler: xli.OnRun(connectionTunnel)},
	}
	return group
}
func connectionSettings(c *xli.Command) (settings.Config, string, error) {
	root := c
	for root.HasParent() {
		root = root.Parent()
	}
	state, err := filepath.Abs(flg.MustGet[string](root, "state"))
	if err != nil {
		return settings.Config{}, "", err
	}
	cfg, err := settings.Load(state)
	return cfg, state, err
}
func connectionList(_ context.Context, c *xli.Command, _ xli.Next) error {
	format := "table"
	for cur := c; cur != nil; {
		if v, set := flg.Get[string](cur, "format"); set {
			format = v
			break
		}
		if !cur.HasParent() {
			break
		}
		cur = cur.Parent()
	}
	if format != "table" && format != "json" {
		return fmt.Errorf("format must be table or json")
	}
	cfg, _, err := connectionSettings(c)
	if err != nil {
		return err
	}
	type row struct {
		Name    string `json:"name"`
		Target  string `json:"target"`
		Default bool   `json:"default"`
		Tunnel  bool   `json:"tunnel"`
	}
	rows := []row{}
	if cfg.Connections != nil {
		for _, name := range cfg.Connections.Names() {
			entry := cfg.Connections.Entries[name]
			e, _ := transport.ParseEndpoint(entry.Resolve("/client-state").Target)
			rows = append(rows, row{name, entry.Target, name == cfg.Connections.DefaultName(), e.Scheme == "ssh"})
		}
	}
	if format == "json" {
		return json.NewEncoder(c.Writer).Encode(rows)
	}
	if len(rows) == 0 {
		_, err = fmt.Fprintln(c.Writer, "No saved connections. Add connections to settings.jsonc with cxz edit.")
		return err
	}
	w := tabwriter.NewWriter(c.Writer, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTARGET\tDEFAULT\tTUNNEL")
	for _, r := range rows {
		def, tunnel := "-", "-"
		if r.Default {
			def = "yes"
		}
		if r.Tunnel {
			tunnel = "ssh"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Target, def, tunnel)
	}
	return w.Flush()
}
func connectionTunnel(ctx context.Context, c *xli.Command, _ xli.Next) error {
	cfg, state, err := connectionSettings(c)
	if err != nil {
		return err
	}
	name := arg.MustGet[string](c, "NAME")
	if cfg.Connections == nil {
		return fmt.Errorf("no saved connections; configure %s using cxz edit", settings.Path(state))
	}
	entry, ok := cfg.Connections.Entries[name]
	if !ok {
		return fmt.Errorf("unknown connection %q; use cxz connection ls", name)
	}
	endpoint, err := transport.ParseEndpoint(entry.Resolve(state).Target)
	if err != nil {
		return err
	}
	local, err := strconv.Atoi(flg.MustGet[string](c, "local-port"))
	if err != nil {
		return fmt.Errorf("--local-port must be an integer from 1 to 65535")
	}
	remote, err := strconv.Atoi(flg.MustGet[string](c, "remote-port"))
	if err != nil {
		return fmt.Errorf("--remote-port must be an integer from 1 to 65535")
	}
	if _, err = endpoint.TunnelArguments(local, remote); err != nil {
		return err
	}
	fmt.Fprintf(c.ErrWriter, "Opening SSH tunnel via %s: 127.0.0.1:%d -> remote 127.0.0.1:%d\nKeep this terminal open; Ctrl+C closes the tunnel.\nFor HTTPS, the browser URL must match the web origin and certificate.\n", name, local, remote)
	return transport.Tunnel(ctx, endpoint, local, remote, c.Writer, c.ErrWriter)
}
