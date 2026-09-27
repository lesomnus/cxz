package versionpin

import (
	"context"
	"encoding/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"strings"
)

type Barrier struct{ Generation, Phase string }

func Pending(root string) bool {
	_, e := os.Stat(filepath.Join(root, "use-barrier.json"))
	if e == nil {
		return true
	}
	p, e := Load(root)
	return e != nil || p.Version != "" && !p.Ready
}
func allowed(ctx context.Context, root, method string) bool {
	if !Pending(root) {
		return true
	}
	name := method[strings.LastIndex(method, "/")+1:]
	switch name {
	case "Get", "List", "Watch", "Events", "History", "Background", "Logs", "Paths", "Memory":
		return true
	}
	if name == "Resume" {
		var b Barrier
		data, e := os.ReadFile(filepath.Join(root, "use-barrier.json"))
		if e == nil && json.Unmarshal(data, &b) == nil && b.Phase == "resuming" {
			m, _ := metadata.FromIncomingContext(ctx)
			v := m.Get("cxz-use-id")
			return len(v) == 1 && v[0] == b.Generation
		}
	}
	return false
}
func Unary(root string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if !allowed(ctx, root, info.FullMethod) {
			return nil, status.Error(codes.Unavailable, "cxz use is restarting this installation; request was not accepted")
		}
		return next(ctx, req)
	}
}
func Stream(root string) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		if !allowed(stream.Context(), root, info.FullMethod) {
			return status.Error(codes.Unavailable, "cxz use is restarting this installation")
		}
		return next(srv, stream)
	}
}
