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
		&flg.Switch{Name: "no-enroll", Brief: "Do not obtain or renew a client certificate over an SSH connection", Default: remoteSwitch(false)},
	}
}
func remoteDefault(v string) *string { return &v }
func remoteSwitch(v bool) *bool      { return &v }

type remoteOptions struct {
	endpoint, tokenFile, session string
	// noEnroll keeps this invocation from obtaining or renewing a certificate
	// over ssh. The automatic path writes durable credentials on a host, so
	// there has to be a way to say no without editing configuration.
	noEnroll bool
}

func runRemote(ctx context.Context, state string, o remoteOptions) error {
	state, err := filepath.Abs(state)
	if err != nil {
		return err
	}
	endpoint, tokenFile, session := o.endpoint, o.tokenFile, o.session
	ctx = versionpin.WithClient(ctx, state)
	cfg, err := settings.Load(state)
	if err != nil {
		return err
	}
	if cfg.Connections != nil {
		if endpoint == "" {
			return runConfigured(ctx, state, cfg, "", session, o)
		}
		if _, ok := cfg.Connections.Entries[endpoint]; ok {
			return runConfigured(ctx, state, cfg, endpoint, session, o)
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
	return transport.DialEndpoint(endpoint, token, nil)
}
func configuredSources(ctx context.Context, state string, cfg settings.Config, o remoteOptions, out io.Writer) []multiclient.Source {
	var sources []multiclient.Source
	for _, name := range cfg.Connections.Names() {
		name, entry := name, cfg.Connections.Entries[name]
		endpoint, _ := transport.ParseEndpoint(entry.Resolve(state).Target)
		open := func() (api.SessionsClient, io.Closer, error) {
			_, conn, err := openConnection(ctx, state, name, entry, !o.noEnroll, out)
			if err != nil {
				return nil, nil, err
			}
			return resourceclient.New(conn), conn, nil
		}
		// The scheme is what this client will use, decided from what it holds
		// rather than from the target alone, so an enrolled ssh connection is
		// reported as the mtls connection it becomes. A connection that falls
		// back to ssh at dial time keeps saying mtls, which changes nothing it
		// is consulted for: both are confidential links.
		scheme, _ := connectionTransport(state, name, entry.Resolve(state).Target)
		if scheme == "" {
			scheme = endpoint.Scheme
		}
		sources = append(sources, multiclient.Source{Name: name, Scheme: scheme, Remote: scheme == "ssh" || scheme == "tcp" || scheme == "mtls", Open: open})
	}
	return sources
}
func runConfigured(ctx context.Context, state string, cfg settings.Config, selected, session string, o remoteOptions) error {
	if selected == "" {
		selected = cfg.Connections.DefaultName()
	}
	client := multiclient.New(ctx, configuredSources(ctx, state, cfg, o, os.Stderr), selected)
	defer client.Close()
	ctx = settings.With(ctx, cfg)
	ctx = tui.WithRecordingDirectory(ctx, filepath.Join(state, "recordings"))
	if session != "" && !strings.Contains(session, "::") {
		session = multiclient.Scope(selected, session)
	}
	return tui.RunSelected(ctx, client, session)
}
