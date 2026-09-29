package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/multiclient"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
	"path/filepath"
	"runtime"
)

func mcpConnect(ctx context.Context, c *xli.Command, r *mcpconfig.Request) (api.SessionsClient, func(), error) {
	root, e := filepath.Abs(flg.MustGet[string](c, "state"))
	if e != nil {
		return nil, nil, e
	}
	cfg, e := settings.Load(root)
	if e != nil {
		return nil, nil, e
	}
	selected := flg.MustGet[string](c, "endpoint")
	tokenFile := flg.MustGet[string](c, "token-file")
	projectSource, project := multiclient.Split(r.Project)
	sessionSource, session := multiclient.Split(r.Session)
	if projectSource != "" {
		selected = projectSource
	}
	if selected == "" && cfg.Connections != nil {
		selected = cfg.Connections.DefaultName()
	}
	if sessionSource != "" && sessionSource != selected {
		return nil, nil, fmt.Errorf("MCP project and session belong to different connections")
	}
	r.Project = project
	r.Session = session
	endpoint := selected
	if cfg.Connections != nil {
		if entry, ok := cfg.Connections.Entries[selected]; ok {
			v := entry.Resolve(root)
			endpoint = v.Target
			tokenFile = v.TokenFile
		}
	}
	if endpoint == "" {
		if runtime.GOOS == "windows" {
			return nil, nil, fmt.Errorf("remote endpoint required")
		}
		endpoint = "local://"
	}
	parsed, e := transport.ParseEndpoint(endpoint)
	if e != nil {
		return nil, nil, e
	}
	token := ""
	if parsed.Scheme == "tcp" {
		token, e = transport.ReadToken(tokenFile)
		if e != nil {
			return nil, nil, e
		}
	}
	conn, e := dialConnection(root, endpoint, token)
	if e != nil {
		return nil, nil, e
	}
	return resourceclient.New(conn), func() { conn.Close() }, nil
}
