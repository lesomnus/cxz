//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/xli/xlitest"
	"google.golang.org/grpc"
)

type eraseStub struct {
	resource.UnimplementedSessionServiceServer
	requests chan string
}

func (s eraseStub) Erase(_ context.Context, r *resource.SessionRef) (*resource.SessionEraseResponse, error) {
	handle := r.GetAlias()
	if handle == "" {
		handle = r.GetRuntimeId()
	}
	s.requests <- handle
	return resource.SessionEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}
func TestSessionEraseReachesTheAPIAndRequiresConfirmation(t *testing.T) {
	root, err := os.MkdirTemp("", "cxz-erase-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err = os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "run", "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := eraseStub{requests: make(chan string, 4)}
	resource.RegisterSessionServiceServer(server, stub)
	go server.Serve(listener)
	defer server.Stop()
	got := xlitest.Run(t, newRoot(root), "session", "erase", "lamp")
	if got.Err == nil || !strings.Contains(got.Err.Error(), "--yes") || len(stub.requests) != 0 {
		t.Fatal("erase ran unconfirmed", got.Err)
	}
	got = xlitest.Run(t, newRoot(root), "session", "erase", "--yes", "lamp")
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	if q := <-stub.requests; q != "lamp" {
		t.Fatal("wrong session", q)
	}
	if !strings.Contains(got.Stdout, "Erased session lamp") {
		t.Fatal(got.Stdout)
	}
	got = xlitest.Run(t, newRoot(root), "session", "erase", "--yes", "--format", "json", "lamp")
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	<-stub.requests
	var reply api.SessionEraseReply
	if err = json.Unmarshal([]byte(got.Stdout), &reply); err != nil || reply.SessionId != "lamp" {
		t.Fatal(got.Stdout, err)
	}
	got = xlitest.Run(t, newRoot(root), "session", "purge", "--yes", "lamp")
	if got.Err == nil || len(stub.requests) != 0 {
		t.Fatal("obsolete purge command still callable", got.Err)
	}
}
