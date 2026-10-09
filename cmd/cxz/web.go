//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/webconfig"
	"github.com/lesomnus/cxz/internal/webui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func webFlags() flg.Flags {
	return flg.Flags{
		stringFlag("config", "Web JSON settings (default: STATE/web.json)", ""),
		stringFlag("listen", "Listen address", ""),
		stringFlag("origin", "Exact browser origin", ""),
		stringFlag("tls-cert", "TLS certificate PEM file", ""),
		stringFlag("tls-key", "TLS private key PEM file", ""),
		stringFlag("access-token-file", "Browser access token file", ""),
	}
}

func webServeFlags() flg.Flags {
	return append(webFlags(), stringFlag("assets-dir", "Built browser UI directory (or CXZ_WEB_ASSETS_DIR)", ""))
}

// loadWebConfig reads the configuration without applying flags, and reports
// which file it came from. An empty path means nothing is configured yet, which
// is what lets `web up` write the default instead of reporting a usage error.
func loadWebConfig(ctx context.Context, c *xli.Command) (webconfig.Config, string, error) {
	if path := flg.MustGet[string](c, "config"); path != "" {
		cfg, err := webconfig.Load(path, false)
		return cfg, path, err
	}
	root := stateFrom(ctx)
	for _, path := range []string{webconfig.DefaultPath(root), filepath.Join(root, "web-installation.json")} {
		if _, err := os.Stat(path); err == nil {
			cfg, err := webconfig.Load(path, false)
			return cfg, path, err
		}
	}
	return webconfig.Config{}, "", nil
}

func overrideWebFlags(c *xli.Command, cfg *webconfig.Config) error {
	for name, target := range map[string]*string{"listen": &cfg.Listen, "origin": &cfg.Origin, "tls-cert": &cfg.Certificate, "tls-key": &cfg.Key, "access-token-file": &cfg.TokenFile} {
		if value := flg.MustGet[string](c, name); value != "" {
			*target = value
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg.Resolve(cwd)
	return nil
}

func readWebConfig(ctx context.Context, c *xli.Command) (webconfig.Config, string, error) {
	cfg, path, err := loadWebConfig(ctx, c)
	if err != nil {
		return cfg, path, err
	}
	return cfg, path, overrideWebFlags(c, &cfg)
}

func webCommand() *xli.Command {
	group := &xli.Command{Name: "web", Brief: "Run the browser gateway for this installation", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) })}
	group.Commands = xli.Commands{
		{Name: "up", Brief: "Create or reconfigure the background gateway container", Flags: webFlags(), Handler: onRun(webUp)},
		{Name: "down", Brief: "Remove the gateway container, keeping its configuration and token", Handler: onRun(func(ctx context.Context, _ *xli.Command) error {
			return installer.UninstallWeb(ctx, stateFrom(ctx))
		})},
		{Name: "status", Brief: "Report the gateway container, its origin and whether it is current", Flags: flg.Flags{stringFlag("config", "Web JSON settings (default: STATE/web.json)", ""), &flg.String{Name: "format", Brief: "Output format: table or json", Default: remoteDefault("table")}}, Handler: onRun(webStatus)},
		{Name: "serve", Brief: "Run the gateway in this terminal; for developing cxz itself", Flags: webServeFlags(), Handler: onRun(serveWeb(false))},
	}
	return group
}

// webUpConfig writes the desktop default when nothing is configured. That
// default is loopback plaintext: the browser treats 127.0.0.1 as a secure
// origin, so a certificate, a hostname and a trust store are all things nobody
// has to produce before opening the page.
func webUpConfig(ctx context.Context, c *xli.Command) (webconfig.Config, error) {
	cfg, path, err := loadWebConfig(ctx, c)
	if err != nil {
		return cfg, err
	}
	if path == "" {
		generated, created, err := webconfig.Generate(stateFrom(ctx))
		if err != nil {
			return cfg, err
		}
		for _, file := range created {
			fmt.Fprintf(c.ErrWriter, "Generated %s\n", file)
		}
		cfg = generated
	}
	return cfg, overrideWebFlags(c, &cfg)
}

func webUp(ctx context.Context, c *xli.Command) error {
	cfg, err := webUpConfig(ctx, c)
	if err != nil {
		return err
	}
	if err = installer.InstallWeb(ctx, stateFrom(ctx), cfg, c.ErrWriter); err != nil {
		return err
	}
	fmt.Fprintf(c.Writer, "Open %s and sign in with the token in %s\n", cfg.Origin, cfg.TokenFile)
	return nil
}

// Status reports what is configured, so it reads the file and no flags: an
// override would describe a gateway that is not the one running.
func webStatus(ctx context.Context, c *xli.Command) error {
	cfg, path, err := loadWebConfig(ctx, c)
	if err != nil {
		return err
	}
	state, err := installer.WebStatus(ctx, stateFrom(ctx), cfg, path)
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
	transport := ""
	if state.Origin != "" {
		transport = map[bool]string{true: "https", false: "http (loopback)"}[state.TLS]
	}
	rows := [][2]string{{"configuration", state.ConfigPath}, {"origin", state.Origin}, {"published", state.Listen},
		{"transport", transport},
		{"token", state.TokenFile}, {"container", state.Container}, {"state", state.State}, {"image", state.Image}, {"manager image", state.ManagerImage}}
	if state.ConfigPath == "" {
		rows[0][1] = "none; cxz web up writes one"
	}
	if !state.TokenPresent && state.TokenFile != "" {
		rows[4][1] = state.TokenFile + " (missing)"
	}
	if state.CertificateExpiry != "" {
		rows = append(rows, [2]string{"certificate expires", state.CertificateExpiry})
	}
	if state.NeedsRefresh {
		rows = append(rows, [2]string{"refresh", "the gateway is older than the Manager; run cxz web up"})
	}
	for _, row := range rows {
		if row[1] == "" {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n", row[0], row[1])
	}
	return w.Flush()
}

// serveWeb runs the gateway in this process. published means the listener is
// reached only through a container port that `cxz web up` published on a
// loopback address, which is what makes a plaintext listener on every interface
// of a container's own network namespace safe. Typing `web serve` binds a host
// interface directly, so that case is refused instead.
func serveWeb(published bool) func(context.Context, *xli.Command) error {
	return func(ctx context.Context, c *xli.Command) error {
		cfg, path, err := readWebConfig(ctx, c)
		if err != nil {
			return err
		}
		// Nothing from a file and nothing from a flag is not a malformed
		// configuration, it is an absent one; say which command writes it.
		if path == "" && cfg.Origin == "" {
			return fmt.Errorf("no web configuration; run cxz web up to write %s, or pass --config", webconfig.DefaultPath(stateFrom(ctx)))
		}
		config, err := cfg.Runtime()
		if err != nil {
			return err
		}
		if !published && config.Certificate == "" {
			host, _, err := net.SplitHostPort(config.Listen)
			if err != nil {
				return fmt.Errorf("web listen: %w", err)
			}
			if !webui.Loopback(host) {
				return fmt.Errorf("a plaintext gateway must listen on a loopback address, not %q; configure an https origin with tls-cert and tls-key to serve a network", config.Listen)
			}
		}
		directory := flg.MustGet[string](c, "assets-dir")
		if directory == "" {
			directory = webui.AssetsDirectory()
		}
		assets, err := webui.Assets(directory)
		if err != nil {
			return err
		}
		conn, err := transport.Dial(stateFrom(ctx))
		if err != nil {
			return err
		}
		defer conn.Close()
		fmt.Fprintf(c.ErrWriter, "cxz web: %s (browser sign-in required; Ctrl+C stops only the gateway)\n", config.Origin)
		return webui.Serve(ctx, config, conn, assets)
	}
}

// internalWebCommand is how the installed container starts the gateway. It is
// not a supported interface: it trusts the publish address cxz web up chose.
func internalWebCommand() *xli.Command {
	return &xli.Command{Name: "_web-serve", Category: "Internal runtime", Brief: "Serve a published gateway container", Flags: webServeFlags(), Handler: onRun(serveWeb(true))}
}
