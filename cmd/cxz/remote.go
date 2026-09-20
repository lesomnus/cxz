package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/multiclient"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"google.golang.org/grpc"
)

func connectCommand() *xli.Command {
	endpoint := os.Getenv("CXZ_ENDPOINT")
	tokenFile := os.Getenv("CXZ_TOKEN_FILE")
	session := ""
	return &xli.Command{Name: "connect", Brief: "Open all configured connections, or connect to NAME / ENDPOINT", Args: arg.Args{&arg.String{Name: "ENDPOINT", Optional: true, Default: &endpoint}}, Flags: flg.Flags{
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
	state, err := filepath.Abs(state)
	if err != nil {
		return err
	}
	cfg, err := settings.Load(state)
	if err != nil {
		return err
	}
	if cfg.Connections != nil {
		if endpoint == "" {
			return runConfigured(ctx, state, cfg, "", session)
		}
		if _, ok := cfg.Connections.Entries[endpoint]; ok {
			return runConfigured(ctx, state, cfg, endpoint, session)
		}
	}
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
	conn, err := dialConnection(state, endpoint, token)
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
	ctx = settings.With(ctx, cfg)
	ctx = tui.WithRecordingDirectory(ctx, filepath.Join(state, "recordings"))
	if e.Scheme == "ssh" || e.Scheme == "tcp" {
		ctx = transport.WithRemote(ctx)
	}
	return tui.RunSelected(ctx, client, session)
}

// local:// follows the installation descriptor, since an installed manager's
// socket is inside Docker rather than necessarily exposed on the host.
func dialConnection(state, endpoint, token string) (*grpc.ClientConn, error) {
	e, err := transport.ParseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if e.Scheme == "local" {
		if e.Address != "" {
			state = e.Address
		}
		return transport.Dial(state)
	}
	return transport.DialEndpoint(endpoint, token)
}
func configuredSources(state string, cfg settings.Config) []multiclient.Source {
	var sources []multiclient.Source
	for _, name := range cfg.Connections.Names() {
		entry := cfg.Connections.Entries[name].Resolve(state)
		endpoint, _ := transport.ParseEndpoint(entry.Target)
		open := func() (api.SessionsClient, io.Closer, error) {
			token := ""
			if endpoint.Scheme == "tcp" {
				var err error
				token, err = transport.ReadToken(entry.TokenFile)
				if err != nil {
					return nil, nil, err
				}
			}
			conn, err := dialConnection(state, entry.Target, token)
			if err != nil {
				return nil, nil, err
			}
			return resourceclient.New(conn), conn, nil
		}
		sources = append(sources, multiclient.Source{Name: name, Remote: endpoint.Scheme == "ssh" || endpoint.Scheme == "tcp", Open: open})
	}
	return sources
}
func runConfigured(ctx context.Context, state string, cfg settings.Config, selected, session string) error {
	if selected == "" {
		selected = cfg.Connections.DefaultName()
	}
	client := multiclient.New(ctx, configuredSources(state, cfg), selected)
	defer client.Close()
	ctx = settings.With(ctx, cfg)
	ctx = tui.WithRecordingDirectory(ctx, filepath.Join(state, "recordings"))
	if session != "" && !strings.Contains(session, "::") {
		session = multiclient.Scope(selected, session)
	}
	return tui.RunSelected(ctx, client, session)
}
