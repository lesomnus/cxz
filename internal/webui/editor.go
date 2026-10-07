package webui

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/lesomnus/cxz/internal/editor"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type editorConnection struct {
	ref   *resource.ProjectRef
	token string
}
type editorProxy struct {
	client    resource.ProjectServiceClient
	mu        sync.Mutex
	connected map[string]editorConnection
}

func (p *editorProxy) connect(ctx context.Context, r *resource.ProjectEditorRequest) (*resource.ProjectEditorReply, error) {
	project, err := p.client.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: boolPointer(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if _, err = editor.BasePath(project.GetRuntimeId()); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	reply, err := p.client.Editor(ctx, r)
	if err != nil {
		return nil, err
	}
	if reply.GetSimulated() || len(reply.GetConnectionToken()) != 64 {
		return nil, status.Error(codes.FailedPrecondition, "invalid Manager editor connection")
	}
	p.mu.Lock()
	p.connected[project.GetRuntimeId()] = editorConnection{ref: proto.Clone(r.GetRef()).(*resource.ProjectRef), token: reply.GetConnectionToken()}
	p.mu.Unlock()
	reply.ClearConnectionToken()
	return reply, nil
}

func (p *editorProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/editor/"), "/", 2)
	if len(parts) != 2 {
		http.Error(w, "editor path required", http.StatusNotFound)
		return
	}
	id := parts[0]
	base, err := editor.BasePath(id)
	if err != nil {
		http.Error(w, "invalid editor project", 400)
		return
	}
	p.mu.Lock()
	connection, ok := p.connected[id]
	p.mu.Unlock()
	if !ok {
		http.Error(w, "connect the editor first", http.StatusConflict)
		return
	}
	// The workbench reads vscode-tkn from document.cookie for its WebSocket
	// control handshake. Restrict that cookie to this editor's path; the gateway
	// still requires the owner's HttpOnly session on every HTTP/WS request.
	if parts[1] == "" {
		http.SetCookie(w, &http.Cookie{Name: "vscode-tkn", Value: connection.token, Path: base + "/", Secure: true, SameSite: http.SameSiteStrictMode})
	}
	transport := &http.Transport{DisableKeepAlives: true, ResponseHeaderTimeout: 15 * time.Second, DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		ctx, cancel := context.WithCancel(r.Context())
		stream, err := p.client.EditorTunnel(ctx)
		if err != nil {
			cancel()
			return nil, err
		}
		if err = stream.Send(resource.ProjectEditorTunnelRequest_builder{Ref: connection.ref}.Build()); err != nil {
			cancel()
			return nil, err
		}
		ready, err := stream.Recv()
		if err != nil {
			cancel()
			return nil, err
		}
		if !ready.GetReady() {
			cancel()
			return nil, fmt.Errorf("editor tunnel not ready")
		}
		return &editorStream{stream: stream, cancel: cancel}, nil
	}}
	defer transport.CloseIdleConnections()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "editor.internal"
			pr.Out.Host = "127.0.0.1:7351"
			query := pr.Out.URL.Query()
			query.Del("tkn")
			pr.Out.URL.RawQuery = query.Encode()
			pr.Out.Header.Del("Cookie")
			pr.Out.AddCookie(&http.Cookie{Name: "vscode-tkn", Value: connection.token})
			pr.Out.Header.Del("Authorization")
			pr.SetXForwarded()
		},
		Transport: transport,
		ModifyResponse: func(response *http.Response) error {
			response.Header.Del("Set-Cookie")
			response.Header.Set("X-Frame-Options", "SAMEORIGIN")
			policy := response.Header.Get("Content-Security-Policy")
			if strings.Contains(policy, "frame-ancestors") {
				policy = strings.ReplaceAll(policy, "frame-ancestors 'none'", "frame-ancestors 'self'")
			} else {
				policy += "; frame-ancestors 'self'"
			}
			response.Header.Set("Content-Security-Policy", policy)
			if location := response.Header.Get("Location"); strings.HasPrefix(location, "/") && location != base && !strings.HasPrefix(location, base+"/") && !strings.HasPrefix(location, base+"?") {
				response.Header.Set("Location", base+location)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "editor connection unavailable; reconnect", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

type editorStream struct {
	stream          grpc.BidiStreamingClient[resource.ProjectEditorTunnelRequest, resource.ProjectEditorTunnelReply]
	cancel          context.CancelFunc
	pending         []byte
	readMu, writeMu sync.Mutex
}

func (s *editorStream) Read(b []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for len(s.pending) == 0 {
		r, err := s.stream.Recv()
		if err != nil {
			return 0, err
		}
		s.pending = r.GetOutput()
	}
	n := copy(b, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}
func (s *editorStream) Write(b []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	total := 0
	for len(b) > 0 {
		n := min(len(b), 65536)
		if err := s.stream.Send(resource.ProjectEditorTunnelRequest_builder{Input: append([]byte(nil), b[:n]...)}.Build()); err != nil {
			return total, err
		}
		total += n
		b = b[n:]
	}
	return total, nil
}
func (s *editorStream) Close() error { s.cancel(); return nil }

type editorAddress string

func (a editorAddress) Network() string                  { return "grpc" }
func (a editorAddress) String() string                   { return string(a) }
func (s *editorStream) LocalAddr() net.Addr              { return editorAddress("gateway") }
func (s *editorStream) RemoteAddr() net.Addr             { return editorAddress("project-editor") }
func (s *editorStream) SetDeadline(time.Time) error      { return nil }
func (s *editorStream) SetReadDeadline(time.Time) error  { return nil }
func (s *editorStream) SetWriteDeadline(time.Time) error { return nil }

var _ io.ReadWriteCloser = (*editorStream)(nil)
