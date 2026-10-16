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
	"github.com/lesomnus/cxz/internal/pki"
)

// Credential is what a link authenticates a file body with, which is the
// credential that link's calls already use: a token on the plaintext surface,
// the enrolled client certificate on mutual TLS, and nothing at all where the
// channel authenticates itself.
type Credential struct {
	Token    string
	Identity *Identity
}

// Use the same local installation/SSH identity, never an invented side port or
// a weaker bearer token.
func AttachmentClient(root string, endpoint Endpoint, cred Credential) *attachmenthttp.Client {
	// A relay is reached as itself rather than through a tunnel to the Manager's
	// socket: its own listener carries the bodies beside the calls, on the one
	// address and with the one credential. Everything else below tunnels to the
	// Manager and speaks HTTP to it directly.
	switch endpoint.Scheme {
	case "tcp":
		origin, err := relayOrigin(endpoint.Address, false)
		if err != nil || cred.Token == "" {
			return nil
		}
		return relayClient(origin, &http.Transport{}, http.Header{"Authorization": {"Bearer " + cred.Token}})
	case "mtls":
		if cred.Identity == nil {
			return nil
		}
		config, err := pki.ClientTLS(cred.Identity.Certificate, cred.Identity.Key, cred.Identity.CA)
		if err != nil {
			return nil
		}
		origin, err := relayOrigin(endpoint.Address, true)
		if err != nil {
			return nil
		}
		return relayClient(origin, &http.Transport{TLSClientConfig: config}, nil)
	}
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
	return relayClient("http://cxz-manager", tr, nil)
}

func relayClient(origin string, tr *http.Transport, header http.Header) *attachmenthttp.Client {
	tr.IdleConnTimeout = 30 * time.Second
	tr.MaxIdleConnsPerHost = 2
	tr.ExpectContinueTimeout = 5 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Minute
	return &attachmenthttp.Client{Origin: origin, Header: header, HTTP: &http.Client{Transport: tr, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return fmt.Errorf("attachment redirects are not allowed")
	}}}
}
