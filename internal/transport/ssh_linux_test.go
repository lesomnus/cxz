//go:build linux

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
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestSSHConnectionHelper(t *testing.T) {
	address := os.Getenv("CXZ_TEST_SSH_BACKEND")
	if address == "" {
		return
	}
	c, err := net.Dial("tcp", address)
	if err != nil {
		os.Exit(2)
	}
	go io.Copy(c, os.Stdin)
	io.Copy(os.Stdout, c)
	c.Close()
	os.Exit(0)
}

func TestSSHGRPCOverProcessPipes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, health.NewServer())
	go server.Serve(ln)
	defer server.Stop()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nexec " + shellArgument(bin) + " -test.run=^TestSSHConnectionHelper$ -- \"$@\"\n"
	if err = os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CXZ_TEST_SSH_BACKEND", ln.Addr().String())
	conn, err := DialEndpoint("ssh://user@fixture:2222?state=%2Ftmp%2Fspace+path", "")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	response, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || response.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatal(response, err)
	}
}

func TestSSHCommandQuoting(t *testing.T) {
	// A path is one shell argument even if it contains shell metacharacters.
	e := Endpoint{Scheme: "ssh", Address: "host", Binary: "cxz", State: "/tmp/a'b;$(printf injected)"}
	args := e.SSHArguments()
	command := args[len(args)-1]
	if strings.Contains(command, "injected)'") == false || !strings.Contains(command, `'"'"'`) {
		t.Fatal(command)
	}
}
