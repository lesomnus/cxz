//go:build !windows

package main

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/webui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func webCommand() *xli.Command {
	return &xli.Command{Name: "web", Brief: "Serve the mobile web client and payday Connect over HTTPS", Flags: flg.Flags{
		stringFlag("listen", "HTTPS listen address", "127.0.0.1:7350"),
		stringFlag("origin", "Exact browser origin, e.g. https://host:7350", ""),
		stringFlag("tls-cert", "TLS certificate PEM file", ""),
		stringFlag("tls-key", "TLS private key PEM file", ""),
		stringFlag("access-token-file", "Browser access token file (32+ random bytes)", ""),
	}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		token, err := transport.ReadToken(flg.MustGet[string](c, "access-token-file"))
		if err != nil {
			return err
		}
		config := webui.Config{Listen: flg.MustGet[string](c, "listen"), Origin: flg.MustGet[string](c, "origin"), Certificate: flg.MustGet[string](c, "tls-cert"), Key: flg.MustGet[string](c, "tls-key"), Token: token}
		if err := config.Validate(); err != nil {
			return err
		}
		conn, err := transport.Dial(stateFrom(ctx))
		if err != nil {
			return err
		}
		defer conn.Close()
		fmt.Fprintf(c.ErrWriter, "cxz web: %s (browser sign-in required; Ctrl+C stops only the gateway)\n", config.Origin)
		return webui.Serve(ctx, config, conn, webui.Assets())
	})}
}
