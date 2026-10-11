package webui

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/lesomnus/cxz/internal/transport"
)

// ManagerProxy forwards HTTP unchanged to the private Manager Connect listener.
// All RPC registration and protocol conversion happen on the Manager's actual
// gRPC server. The frontend server owns only browser authentication and bridges.
func ManagerProxy(root string) (http.Handler, func()) {
	upstream := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return transport.LocalConnectConnection(ctx, root)
		},
		IdleConnTimeout:     60 * time.Second,
		MaxIdleConnsPerHost: 16,
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = "http"
			r.Out.URL.Host = "cxz-manager"
			r.Out.Host = "cxz-manager"
			r.Out.Header.Del("Cookie")
			r.Out.Header.Del("Authorization")
			r.Out.Header.Del("Origin")
		},
		Transport:     upstream,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			if r.Context().Err() == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				}{Code: "unavailable", Message: "Manager Connect endpoint unavailable; update and restart the Manager"})
			}
		},
	}
	return proxy, upstream.CloseIdleConnections
}

func boolPointer(v bool) *bool { return &v }
