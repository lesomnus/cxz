package transport

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestEndpointValidation(t *testing.T) {
	for _, raw := range []string{"ssh://user@host:2222?state=%2Ftmp%2Fmy+state&binary=%2Fopt%2Fcxz", "ssh://my-host", "ssh://user@[::1]:22", "tcp://127.0.0.1:7349", "tcp://[::1]:7349"} {
		if _, err := ParseEndpoint(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"", "https://host", "ssh://-oProxyCommand=x", "ssh://user:password@host", "ssh://host?exec=touch", "ssh://host?state=a&state=b", "ssh://host/path", "tcp://host", "tcp://host:0", "tcp://host:99999", "tcp://user@host:1234", "tcp://host:1234?token=x", "ssh://host#x", "ssh://host?binary=%00"} {
		if _, err := ParseEndpoint(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	e, err := ParseEndpoint("ssh://user@host:2222?state=%2Ftmp%2Fa%27b%3B%24%28echo+x%29")
	if err != nil {
		t.Fatal(err)
	}
	args := e.SSHArguments()
	if !strings.Contains(args[len(args)-1], `'"'"'`) || args[len(args)-2] != "host" {
		t.Fatal(args)
	}
	if _, err := DialEndpoint("tcp://127.0.0.1:7349", ""); err == nil {
		t.Fatal("missing token accepted")
	}
}

func TestReadRemoteToken(t *testing.T) {
	p := filepath.Join(t.TempDir(), "token")
	for _, value := range []string{"", "short", strings.Repeat("x", 4097), strings.Repeat("x", 32) + "\nsecond"} {
		os.WriteFile(p, []byte(value), 0600)
		if _, err := ReadToken(p); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	want := strings.Repeat("a", 64)
	os.WriteFile(p, []byte(want+"\n"), 0600)
	if got, err := ReadToken(p); err != nil || got != want {
		t.Fatal(got, err)
	}
}

func TestAuthenticatedTCPProxyUnaryAndWatch(t *testing.T) {
	backend := grpc.NewServer()
	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(backend, h)
	buf := bufconn.Listen(1024 * 1024)
	go backend.Serve(buf)
	defer backend.Stop()
	upstream, err := grpc.NewClient("passthrough:///fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return buf.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()
	token := strings.Repeat("secret", 8)
	proxy := authenticatedProxy(upstream, token)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go proxy.Serve(ln)
	defer proxy.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, credential := range []string{"wrong", token} {
		conn, err := DialEndpoint("tcp://"+ln.Addr().String(), credential)
		if err != nil {
			t.Fatal(err)
		}
		client := healthpb.NewHealthClient(conn)
		response, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		if credential != token {
			if status.Code(err) != codes.Unauthenticated {
				t.Fatal("bad unary authentication", err)
			}
			watch, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
			if err == nil {
				_, err = watch.Recv()
			}
			if status.Code(err) != codes.Unauthenticated {
				t.Fatal("bad stream authentication", err)
			}
		} else {
			if err != nil || response.Status != healthpb.HealthCheckResponse_SERVING {
				t.Fatal(response, err)
			}
			watchCtx, stop := context.WithCancel(ctx)
			watch, err := client.Watch(watchCtx, &healthpb.HealthCheckRequest{})
			if err != nil {
				t.Fatal(err)
			}
			response, err = watch.Recv()
			if err != nil || response.Status != healthpb.HealthCheckResponse_SERVING {
				t.Fatal(response, err)
			}
			h.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
			response, err = watch.Recv()
			if err != nil || response.Status != healthpb.HealthCheckResponse_NOT_SERVING {
				t.Fatal(response, err)
			}
			stop()
			_, err = watch.Recv()
			if err == nil || err == io.EOF {
				t.Fatal("stream cancellation lost", err)
			}
			_, err = client.Check(ctx, &healthpb.HealthCheckRequest{Service: "missing"})
			if status.Code(err) != codes.NotFound {
				t.Fatal("upstream status lost", err)
			}
		}
		conn.Close()
	}
}
