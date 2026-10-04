//go:build !windows

package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/webconfig"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/webui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func webFlags() flg.Flags {
	return flg.Flags{
		stringFlag("config", "Web JSON settings (default: STATE/web.json)", ""),
		stringFlag("listen", "HTTPS listen address", ""),
		stringFlag("origin", "Exact browser HTTPS origin", ""),
		stringFlag("tls-cert", "TLS certificate PEM file", ""),
		stringFlag("tls-key", "TLS private key PEM file", ""),
		stringFlag("access-token-file", "Browser access token file", ""),
	}
}
func readWebConfig(ctx context.Context, c *xli.Command) (webconfig.Config, error) {
	path := flg.MustGet[string](c, "config")
	optional := path == ""
	if optional {
		path = filepath.Join(stateFrom(ctx), "web.json")
	}
	cfg, err := webconfig.Load(path, optional)
	if err != nil {
		return cfg, err
	}
	for name, target := range map[string]*string{"listen": &cfg.Listen, "origin": &cfg.Origin, "tls-cert": &cfg.Certificate, "tls-key": &cfg.Key, "access-token-file": &cfg.TokenFile} {
		if value := flg.MustGet[string](c, name); value != "" {
			*target = value
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return cfg, err
	}
	cfg.Resolve(cwd)
	return cfg, nil
}
func installWebCommand() *xli.Command {
	return &xli.Command{Name: "web", Brief: "Install or reconfigure the background HTTPS web container", Flags: webFlags(), Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		cfg, err := readWebConfig(ctx, c)
		if err != nil {
			return err
		}
		return installer.InstallWeb(ctx, stateFrom(ctx), cfg, c.ErrWriter)
	})}
}
func webCommand() *xli.Command {
	return &xli.Command{Name: "web", Brief: "Serve the mobile web client and payday Connect over HTTPS", Flags: webFlags(), Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		cfg, err := readWebConfig(ctx, c)
		if err != nil {
			return err
		}
		config, err := cfg.Runtime()
		if err != nil {
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

func uninstallWebCommand() *xli.Command {
	return &xli.Command{Name: "web", Brief: "Remove the web container, retaining configuration and credentials", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		return installer.UninstallWeb(ctx, stateFrom(ctx))
	})}
}
