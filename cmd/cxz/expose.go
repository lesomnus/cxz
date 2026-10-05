//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

// expose is where an installation decides to be reachable. The bare command is
// the original plaintext relay, kept because a tunnel is still a reasonable
// place to put one; the subcommands manage the mutual-TLS relay, which is a
// container with a lifecycle rather than a terminal somebody must keep open.
//
// All of it is host-local: these commands talk to Docker and to the manager's
// own state. Nothing here reaches another machine, which is why they are named
// after what they do to this one.
func exposeCommand() *xli.Command {
	c := &xli.Command{Name: "expose", Brief: "Forward the installed daemon over authenticated plaintext TCP (VPN/tunnel)",
		Flags: flg.Flags{stringFlag("listen", "TCP listen endpoint", "tcp://127.0.0.1:7349"), stringFlag("token-file", "Remote access token file (32+ bytes)", os.Getenv("CXZ_TOKEN_FILE"))},
		Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			token, err := transport.ReadToken(flg.MustGet[string](c, "token-file"))
			if err != nil {
				return err
			}
			return transport.Expose(ctx, stateFrom(ctx), flg.MustGet[string](c, "listen"), token, c.ErrWriter)
		})}
	c.Commands = xli.Commands{
		{Name: "up", Brief: "Create or reconfigure the background mutual-TLS relay", Flags: flg.Flags{stringFlag("listen", "Published address for the relay", "0.0.0.0:7349")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			cfg := installer.RelayConfig{Listen: flg.MustGet[string](c, "listen")}
			if err := installer.InstallRelay(ctx, stateFrom(ctx), cfg, c.ErrWriter); err != nil {
				return err
			}
			fmt.Fprintf(c.Writer, "mtls://%s\n", cfg.Listen)
			return nil
		})},
		{Name: "down", Brief: "Remove the relay container, keeping the installation root and issued certificates", Handler: onRun(func(ctx context.Context, _ *xli.Command) error {
			return installer.UninstallRelay(ctx, stateFrom(ctx))
		})},
		{Name: "status", Brief: "Report the relay, the installation root and the clients it has enrolled", Flags: flg.Flags{&flg.String{Name: "format", Brief: "Output format: table or json", Default: remoteDefault("table")}}, Handler: onRun(exposeStatus)},
		{Name: "ca", Brief: "Print this installation's root certificate for a client to pin", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			out, err := manager(ctx, nil, "_pki", "ca")
			if err != nil {
				return err
			}
			_, err = c.Writer.Write(out)
			return err
		})},
		{Name: "sign", Brief: "Sign a client certificate request read from stdin", Flags: flg.Flags{stringFlag("label", "Name recorded for this client", "")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			csr, err := io.ReadAll(io.LimitReader(c.ReadCloser, 1<<16))
			if err != nil {
				return err
			}
			out, err := manager(ctx, csr, "_pki", "sign", "--label", flg.MustGet[string](c, "label"))
			if err != nil {
				return err
			}
			_, err = c.Writer.Write(out)
			return err
		})},
		{Name: "revoke", Brief: "Refuse a client certificate this installation issued", Args: arg.Args{stringArg("SERIAL", false)}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			if _, err := manager(ctx, nil, "_pki", "revoke", arg.MustGet[string](c, "SERIAL")); err != nil {
				return err
			}
			fmt.Fprintf(c.ErrWriter, "revoked %s; the relay refuses it from the next connection, with no restart\n", arg.MustGet[string](c, "SERIAL"))
			return nil
		})},
	}
	return c
}

// manager runs a command inside the manager container. Everything touching the
// installation root goes through here, so the private key is reachable from one
// process on one machine and never from a client or from the network.
func manager(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	v, err := transport.Load(stateFrom(ctx))
	if err != nil {
		return nil, fmt.Errorf("install the Manager first: %w", err)
	}
	full := append([]string{"exec", "-i", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz"}, args...)
	if stdin == nil {
		full = append([]string{"exec", v.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz"}, args...)
	}
	return dockerx.Output(ctx, bytes.NewReader(stdin), full...)
}

func exposeStatus(ctx context.Context, c *xli.Command) error {
	state, err := installer.RelayStatus(ctx, stateFrom(ctx))
	if err != nil {
		return err
	}
	format := flg.MustGet[string](c, "format")
	if format == "json" {
		return json.NewEncoder(c.Writer).Encode(state)
	}
	if format != "table" {
		return fmt.Errorf("format must be table or json")
	}
	w := tabwriter.NewWriter(c.Writer, 0, 4, 2, ' ', 0)
	rows := [][2]string{{"listen", state.Listen}, {"container", state.Container}, {"state", state.State},
		{"image", state.Image}, {"manager image", state.ManagerImage}, {"root", state.Root}}
	if state.Listen == "" {
		rows[0][1] = "none; cxz expose up starts a relay"
	}
	if len(state.Clients) > 0 {
		rows = append(rows, [2]string{"clients", fmt.Sprintf("%d enrolled", len(state.Clients))})
	}
	if state.NeedsRefresh {
		rows = append(rows, [2]string{"refresh", "the relay is older than the Manager; run cxz expose up"})
	}
	for _, row := range rows {
		if row[1] == "" {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n", row[0], row[1])
	}
	return w.Flush()
}
