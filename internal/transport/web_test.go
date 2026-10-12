//go:build !windows

package transport

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestManagerConnectUsesRegisteredServicesAndInterceptors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	g := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		calls.Add(1)
		if req.(*healthpb.HealthCheckRequest).GetService() == "denied" {
			return nil, status.Error(codes.PermissionDenied, "same Manager policy")
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs("manager-header", "present"))
		grpc.SetTrailer(ctx, metadata.Pairs("manager-trailer", "present"))
		return handler(ctx, req)
	}))
	defer g.Stop()
	healthpb.RegisterHealthServer(g, health.NewServer())
	listener := bufconn.Listen(1 << 20)
	go g.Serve(listener)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stop, err := StartConnect(ctx, root, g)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	info, err := os.Stat(ConnectSocket(root))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private socket permissions", err)
	}
	upstream := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return LocalConnectConnection(ctx, root) }}
	defer upstream.CloseIdleConnections()
	client := connect.NewClient[healthpb.HealthCheckRequest, healthpb.HealthCheckResponse](&http.Client{Transport: upstream}, "http://manager/grpc.health.v1.Health/Check", connect.WithProtoJSON())
	reply, err := client.CallUnary(t.Context(), connect.NewRequest(&healthpb.HealthCheckRequest{}))
	if err != nil || reply.Msg.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatal("service discovered from the actual server", err)
	}
	if reply.Header().Get("Manager-Header") != "present" || reply.Trailer().Get("Manager-Trailer") != "present" {
		t.Fatal("Manager metadata lost", reply.Header(), reply.Trailer())
	}
	_, err = client.CallUnary(t.Context(), connect.NewRequest(&healthpb.HealthCheckRequest{Service: "denied"}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("interceptor bypassed", err)
	}
	// The original native listener remains independent of HTTP/Connect.
	native, err := grpc.NewClient("passthrough:///fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	if _, err := healthpb.NewHealthClient(native).Check(t.Context(), &healthpb.HealthCheckRequest{}); err != nil || calls.Load() != 3 {
		t.Fatal("native gRPC/interceptor changed", err, calls.Load())
	}
	cancel()
	stop()
	if _, err := os.Stat(ConnectSocket(root)); !os.IsNotExist(err) {
		t.Fatal("socket not removed on shutdown", err)
	}
	stop() // Closing is idempotent.
}

func TestManagerConnectRefusesRegularFileAndCanRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	path := ConnectSocket(root)
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	defer g.Stop()
	healthpb.RegisterHealthServer(g, health.NewServer())
	if _, err := StartConnect(t.Context(), root, g); err == nil {
		t.Fatal("regular file removed")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
		t.Fatal("file changed", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	stale.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stop, err := StartConnect(ctx, root, g)
	if err != nil {
		t.Fatal("stale socket restart", err)
	}
	stop()
}
