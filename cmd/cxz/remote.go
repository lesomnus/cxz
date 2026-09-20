package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func connectCommand() *xli.Command {
	endpoint := os.Getenv("CXZ_ENDPOINT")
	tokenFile := os.Getenv("CXZ_TOKEN_FILE")
	session := ""
	return &xli.Command{Name: "connect", Brief: "Open TUI on a remote Linux daemon via ssh:// or tcp://", Args: arg.Args{&arg.String{Name: "ENDPOINT", Optional: true, Default: &endpoint}}, Flags: flg.Flags{
		&flg.String{Name: "token-file", Brief: "TCP bearer token file (or CXZ_TOKEN_FILE)", Default: &tokenFile},
		&flg.String{Name: "session", Brief: "Initially select a remote session ID or alias", Default: &session},
	}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		root := c
		for root.HasParent() {
			root = root.Parent()
		}
		return runRemote(ctx, flg.MustGet[string](root, "state"), arg.MustGet[string](c, "ENDPOINT"), flg.MustGet[string](c, "token-file"), flg.MustGet[string](c, "session"))
	})}
}

func runRemote(ctx context.Context, state, endpoint, tokenFile, session string) error {
	if endpoint == "" {
		return fmt.Errorf("remote endpoint required: cxz connect ssh://user@host (or set CXZ_ENDPOINT)")
	}
	e, err := transport.ParseEndpoint(endpoint)
	if err != nil {
		return err
	}
	token := ""
	if e.Scheme == "tcp" {
		token, err = transport.ReadToken(tokenFile)
		if err != nil {
			return err
		}
	}
	conn, err := transport.DialEndpoint(endpoint, token)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	check, cancel := context.WithTimeout(ctx, 15*time.Second)
	_, err = client.List(check, &api.Empty{})
	cancel()
	if err != nil {
		return fmt.Errorf("connect to remote cxz: %w", err)
	}
	cfg, err := settings.Load(state)
	if err != nil {
		return err
	}
	ctx = settings.With(ctx, cfg)
	ctx = tui.WithRecordingDirectory(ctx, filepath.Join(state, "recordings"))
	ctx = transport.WithRemote(ctx)
	return tui.RunSelected(ctx, client, session)
}
