package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/attachmenthttp"
)

// Use the same local installation/SSH identity, never an invented side port or
// a weaker bearer token. TCP/mTLS relays retain RPC until they expose raw HTTP.
func AttachmentClient(root string, endpoint Endpoint) *attachmenthttp.Client {
	var dial func(context.Context) (net.Conn, error)
	switch endpoint.Scheme {
	case "", "local":
		if endpoint.Address != "" {
			root = endpoint.Address
		}
		dial = func(ctx context.Context) (net.Conn, error) { return LocalConnectConnection(ctx, root) }
	case "unix":
		if filepath.Base(endpoint.Address) != "daemon.sock" {
			return nil
		}
		path := filepath.Join(filepath.Dir(endpoint.Address), "web.sock")
		dial = func(ctx context.Context) (net.Conn, error) { return (&net.Dialer{}).DialContext(ctx, "unix", path) }
	case "ssh":
		dial = func(ctx context.Context) (net.Conn, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			command := exec.Command("ssh", endpoint.SSHCommand("_connect-http")...)
			diagnostics := &connectionDiagnostics{}
			command.Stderr = diagnostics
			conn, err := commandConnection(command)
			if err != nil {
				return nil, err
			}
			return &diagnosticConnection{Conn: conn, diagnostics: diagnostics}, nil
		}
	default:
		return nil
	}
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx) }, IdleConnTimeout: 30 * time.Second, MaxIdleConnsPerHost: 2, ExpectContinueTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Minute}
	return &attachmenthttp.Client{Origin: "http://cxz-manager", HTTP: &http.Client{Transport: tr, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return fmt.Errorf("attachment redirects are not allowed")
	}}}
}
