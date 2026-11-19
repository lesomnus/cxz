//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/configeditor"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
)

func hostConfigurationEditor(root string) configeditor.Editor {
	return configeditor.Editor{
		Root: root,
		Publish: func(ctx context.Context, spec projectconfig.Spec) error {
			conn, err := transport.Dial(root)
			if err != nil {
				return err
			}
			defer conn.Close()
			client := resourceclient.New(conn)
			defer client.Close()
			_, err = publishDevcontainerSpec(ctx, client, spec)
			return err
		},
	}
}

func configurationHandler(root string, published bool) http.Handler {
	if !published {
		return hostConfigurationEditor(root)
	}
	socket := os.Getenv(configeditor.SocketEnv)
	if socket == "" {
		return nil
	}
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(&url.URL{Scheme: "http", Host: "configuration-editor"})
			r.Out.Header.Del("Cookie")
			r.Out.Header.Del("Authorization")
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
			},
			ResponseHeaderTimeout: 40 * time.Second,
			DisableKeepAlives:     true,
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "The host editor is unavailable. Run cxz web config-editor on this installation's host.", http.StatusServiceUnavailable)
		},
	}
}

func serveConfigurationEditor(ctx context.Context, c *xli.Command) error {
	root := stateFrom(ctx)
	if _, err := transport.Load(root); err != nil {
		return fmt.Errorf("install the Manager first: %w", err)
	}
	dir, err := configeditor.PrepareSocketDir(root)
	if err != nil {
		return err
	}
	lock, err := core.Lock(filepath.Join(dir, "serve.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	path := filepath.Join(dir, "host.sock")
	// Remove only a previous socket. The parent directory is private.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("%s is not a socket", path)
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer ln.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	server := &http.Server{
		Handler:           hostConfigurationEditor(root),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
	}
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	fmt.Fprintln(c.ErrWriter, "Serving installation configuration editor on", path)
	if err := server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
