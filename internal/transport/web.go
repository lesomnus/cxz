package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/lesomnus/payday/config"
	pdweb "github.com/lesomnus/payday/web"
	"google.golang.org/grpc"
)

// ConnectSocket is the Manager's private HTTP/Connect listener, next to its native
// gRPC socket. The web container already mounts this run directory read-only.
func ConnectSocket(root string) string { return filepath.Join(root, "run", "web.sock") }

// StartConnect exposes the actual registered server, including its interceptors,
// through payday. No service implementation or method forwarding is repeated.
// Native gRPC continues to use its existing listener and transport.
func StartConnect(ctx context.Context, root string, g *grpc.Server) (func(), error) {
	handler, err := pdweb.New(config.HttpConfig{AllowWeb: true}, g)
	if err != nil {
		return nil, err
	}
	path := ConnectSocket(root)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("Manager Connect path is not a socket: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	var once sync.Once
	closeServer := func() {
		once.Do(func() {
			_ = server.Close()
			_ = listener.Close()
			<-done
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			closeServer()
		case <-done:
		}
	}()
	return closeServer, nil
}

// LocalConnectConnection has the same installed-container/local-daemon resolution
// as native gRPC, but reaches the Manager's HTTP socket. Bytes remain HTTP; this
// bridge does not decode protobuf or dispatch RPC methods.
func LocalConnectConnection(ctx context.Context, root string) (net.Conn, error) {
	install, err := Load(root)
	if err == nil {
		return commandConnection(exec.Command("docker", "exec", "-i", install.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_web-bridge"))
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", ConnectSocket(root))
}

func WebBridge(root string) error {
	return bridgeSocket(ConnectSocket(root))
}
