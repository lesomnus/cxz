//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/pki"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

// The installation's root lives with the manager's state, not on the host and
// not in any container that listens on a network. Everything that needs the
// private half runs here, inside the manager, reached through docker exec by
// the host commands in expose.go -- so the key has no path out.
func pkiDir(ctx context.Context) string { return filepath.Join(stateFrom(ctx), "pki") }

func pkiInternalCommands() xli.Commands {
	group := &xli.Command{Name: "_pki", Category: "Internal runtime", Brief: "Sign and report this installation's client certificates", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) })}
	group.Commands = xli.Commands{
		{Name: "ca", Brief: "Print the installation root, creating it when absent", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			a, err := pki.EnsureCA(pkiDir(ctx))
			if err != nil {
				return err
			}
			_, err = c.Writer.Write(a.CertificatePEM())
			return err
		})},
		{Name: "sign", Brief: "Sign a client certificate request read from stdin", Flags: flg.Flags{stringFlag("label", "Name recorded for this client", "")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			a, err := pki.EnsureCA(pkiDir(ctx))
			if err != nil {
				return err
			}
			csr, err := io.ReadAll(io.LimitReader(c.ReadCloser, 1<<16))
			if err != nil {
				return err
			}
			cert, record, err := a.SignClient(csr, flg.MustGet[string](c, "label"), pki.ClientLife)
			if err != nil {
				return err
			}
			fmt.Fprintf(c.ErrWriter, "issued %s to %s until %s\n", record.Serial, record.Label, record.Expires.UTC().Format("2006-01-02"))
			_, err = c.Writer.Write(cert)
			return err
		})},
		{Name: "server", Brief: "Create or refresh the relay's own certificate", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			a, err := pki.EnsureCA(pkiDir(ctx))
			if err != nil {
				return err
			}
			return a.EnsureServer()
		})},
		{Name: "revoke", Brief: "Refuse a previously issued client certificate", Args: arg.Args{stringArg("SERIAL", false)}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			a, err := pki.EnsureCA(pkiDir(ctx))
			if err != nil {
				return err
			}
			return a.Revoke(arg.MustGet[string](c, "SERIAL"))
		})},
		{Name: "issued", Brief: "Report issued client certificates as JSON", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			a, err := pki.EnsureCA(pkiDir(ctx))
			if err != nil {
				return err
			}
			records, err := a.Issued()
			if err != nil {
				return err
			}
			revoked, err := pki.Revoked(pki.ServerDir(pkiDir(ctx)))
			if err != nil {
				return err
			}
			type row struct {
				pki.Record
				Revoked bool `json:"revoked"`
			}
			out := []row{}
			for _, r := range records {
				out = append(out, row{Record: r, Revoked: revoked[r.Serial]})
			}
			return json.NewEncoder(c.Writer).Encode(map[string]any{"root": a.Fingerprint(), "issued": out})
		})},
	}
	// The relay runs this. It is separate from the public `expose` because the
	// public one binds a host address a person chose, while this one binds every
	// interface of a container whose published port cxz decided -- and because
	// the relay must never be reachable without the certificate check.
	serve := &xli.Command{Name: "_expose-serve", Category: "Internal runtime", Brief: "Serve the mutual-TLS relay inside its container", Flags: flg.Flags{stringFlag("listen", "Listen address", "0.0.0.0:7349")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		return transport.ExposeMutual(ctx, stateFrom(ctx), flg.MustGet[string](c, "listen"), pki.ServerDir(pkiDir(ctx)), c.ErrWriter)
	})}
	// Readiness is asked from inside the relay's own container, because the
	// command that created it may not be on the machine the port was published
	// to -- a remote Docker engine is a supported arrangement, and there the
	// installer's own loopback is a different loopback.
	check := &xli.Command{Name: "_expose-check", Category: "Internal runtime", Brief: "Connect to a relay address, optionally with a client identity",
		Flags: flg.Flags{stringFlag("address", "host:port to connect to", ""), stringFlag("identity", "Directory holding client.crt, client.key and ca.crt", "")},
		Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			return checkRelay(ctx, flg.MustGet[string](c, "address"), flg.MustGet[string](c, "identity"), c.Writer)
		})}
	return xli.Commands{group, serve, check}
}
