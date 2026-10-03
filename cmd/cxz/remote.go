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
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/xli/flg"
	"google.golang.org/grpc"
)

func remoteFlags() flg.Flags {
	return flg.Flags{
		&flg.String{Name: "endpoint", Brief: "Remote connection name or endpoint (or CXZ_ENDPOINT)", Default: remoteDefault(os.Getenv("CXZ_ENDPOINT"))},
		&flg.String{Name: "token-file", Brief: "TCP bearer token file (or CXZ_TOKEN_FILE)", Default: remoteDefault(os.Getenv("CXZ_TOKEN_FILE"))},
		&flg.String{Name: "session", Brief: "Initially selected remote session ID or alias", Default: remoteDefault("")},
	}
}
func remoteDefault(v string) *string { return &v }

func runRemote(ctx context.Context, state, endpoint, tokenFile, session string) error {
	state, err := filepath.Abs(state)
	if err != nil {
		return err
	}
	ctx = versionpin.WithClient(ctx, state)
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
		return fmt.Errorf("remote endpoint required: cxz --endpoint ssh://user@host (or set CXZ_ENDPOINT)")
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
	ctx = transport.WithScheme(ctx, e.Scheme)
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
		sources = append(sources, multiclient.Source{Name: name, Scheme: endpoint.Scheme, Remote: endpoint.Scheme == "ssh" || endpoint.Scheme == "tcp", Open: open})
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
