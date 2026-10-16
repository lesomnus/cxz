package transport

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/lesomnus/cxz/internal/attachmenthttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// relayHandler puts a relay's calls and its file bodies on one listener.
//
// Both are HTTP/2, so there is nothing to add but a route: `/attachments/` is
// forwarded to the Manager's own raw endpoint, and everything else is the same
// transparent gRPC proxy as before -- a *grpc.Server is an http.Handler, so it
// keeps forwarding methods it does not know, which is what lets a relay serve a
// client newer than itself.
//
// No second port, and no tunnel RPC: wrapping a body in protobuf messages again
// is the cost this exists to remove.
func relayHandler(g *grpc.Server, root string, authorize func(*http.Request) error) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(attachmenthttp.Prefix, attachmentRelay(root, authorize))
	mux.Handle("/", g)
	return mux
}

// relayServer is the relay's HTTP server. A plaintext listener needs h2c to
// carry gRPC at all; a TLS one negotiates h2 in the handshake, so it is served
// with [http.Server.ServeTLS] and gets HTTP/2 without being told.
func relayServer(h http.Handler, plaintext bool) *http.Server {
	if plaintext {
		h = h2c.NewHandler(h, &http2.Server{})
	}
	// No ReadTimeout or WriteTimeout: a call may be a long-lived stream and a
	// body may be a large file. The attachment handler bounds its own read, and
	// a header that never finishes is what ReadHeaderTimeout is for.
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 32 << 10}
}

// attachmentRelay forwards a file body to the Manager's HTTP socket.
//
// Authorization happens here, before the body is read, because a relay that
// streams a megabyte and then refuses it has already spent the transfer. The
// Manager behind this applies its own guards to the same request.
func attachmentRelay(root string, authorize func(*http.Request) error) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = "http"
			r.Out.URL.Host = "cxz-manager"
			r.Out.Host = "cxz-manager"
			// The peer's own credential does not travel inward. The Manager's
			// socket is reached by holding it, and what it authorizes is this
			// relay rather than the client this relay already accepted.
			r.Out.Header.Del("Authorization")
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return LocalConnectConnection(ctx, root)
			},
			IdleConnTimeout:       30 * time.Second,
			MaxIdleConnsPerHost:   2,
			ExpectContinueTimeout: 5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Minute,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() != nil {
				attachmenthttp.Error(w, status.FromContextError(r.Context().Err()).Err())
				return
			}
			// A Manager without the endpoint, or one that is not running, is not
			// this relay's failure to describe as its own.
			attachmenthttp.Error(w, status.Errorf(codes.Unavailable, "attachment endpoint unreachable: %v", err))
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := authorize(r); err != nil {
			attachmenthttp.Error(w, err)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

// RequireTokenHeader is [RequireToken] for a request that arrived as itself.
// The credential is the same one and in the same place: gRPC metadata over
// HTTP/2 is an HTTP header, so a token that authorizes a call authorizes a body
// without a second thing to manage.
func RequireTokenHeader(h http.Header, token string) error {
	if token == "" || subtle.ConstantTimeCompare([]byte(h.Get("Authorization")), []byte("Bearer "+token)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid remote access token")
	}
	return nil
}

// relayOrigin is where a client posts a body for a relay it reaches directly.
func relayOrigin(address string, secure bool) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("relay address: %w", err)
	}
	scheme := "http"
	if secure {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, port)}).String(), nil
}
