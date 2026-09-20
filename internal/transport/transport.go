package transport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Installation struct {
	Owner         string `json:"owner"`
	Container     string `json:"container"`
	Image         string `json:"image"`
	PreviousImage string `json:"previous_image,omitempty"`
	WorkspaceRoot string `json:"workspace_root"`
	StateVolume   string `json:"state_volume"`
	ToolsVolume   string `json:"tools_volume"`
}

func Load(root string) (Installation, error) {
	var v Installation
	b, e := os.ReadFile(filepath.Join(root, "installation.json"))
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func Socket(root string) string { return filepath.Join(root, "run", "daemon.sock") }
func Dial(root string) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///cxz", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return LocalConnection(ctx, root)
	}))
}

type pipeConn struct {
	io.Reader
	io.Writer
	cmd  *exec.Cmd
	once sync.Once
}

func (p *pipeConn) Close() error {
	p.once.Do(func() { p.Writer.(io.Closer).Close(); p.Reader.(io.Closer).Close(); p.cmd.Process.Kill(); p.cmd.Wait() })
	return nil
}

type address string

func (a address) Network() string                    { return "docker-exec" }
func (a address) String() string                     { return string(a) }
func (p *pipeConn) LocalAddr() net.Addr              { return address("client") }
func (p *pipeConn) RemoteAddr() net.Addr             { return address("cxz") }
func (p *pipeConn) SetDeadline(time.Time) error      { return nil }
func (p *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (p *pipeConn) SetWriteDeadline(time.Time) error { return nil }
func Bridge(root string) error {
	c, e := net.Dial("unix", Socket(root))
	if e != nil {
		return e
	}
	defer c.Close()
	done := make(chan error, 2)
	go func() { _, e := io.Copy(c, os.Stdin); done <- e }()
	go func() { _, e := io.Copy(os.Stdout, c); done <- e }()
	return <-done
}
func Remote(endpoint, token string) (*grpc.ClientConn, error) {
	return grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)), grpc.WithUnaryInterceptor(func(ctx context.Context, m string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoke(metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), m, req, reply, cc, opts...)
	}), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, m string, stream grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return stream(metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), desc, cc, m, opts...)
	}))
}
func RequireToken(ctx context.Context, token string) error {
	v, ok := metadata.FromIncomingContext(ctx)
	if token == "" || !ok || len(v.Get("authorization")) != 1 || subtle.ConstantTimeCompare([]byte(v.Get("authorization")[0]), []byte("Bearer "+token)) != 1 {
		return fmt.Errorf("invalid project capability")
	}
	return nil
}
