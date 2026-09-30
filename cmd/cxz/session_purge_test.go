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

	"github.com/lesomnus/cxz/internal/sessionpurge"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/xli/xlitest"
	"google.golang.org/grpc"
)

type purgeStub struct {
	resource.UnimplementedProjectServiceServer
	requests chan sessionpurge.Request
	reply    sessionpurge.Reply
}

func (s purgeStub) Docker(_ context.Context, r *resource.DockerRequest) (*resource.DockerReply, error) {
	var q sessionpurge.Request
	if err := json.Unmarshal(r.GetSpec(), &q); err != nil {
		return nil, err
	}
	s.requests <- q
	reply := s.reply
	reply.DryRun = q.DryRun
	b, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	return resource.DockerReply_builder{Status: ptr(string(b))}.Build(), nil
}

// The connection flags live on the root command, so a command nested two levels
// under it has to reach them there. Reading them off itself panics rather than
// falling back on a default, which is how this went unnoticed before.
func TestSessionPurgeReachesTheAPIThroughNestedCommands(t *testing.T) {
	root, err := os.MkdirTemp("", "cxz-purge-")
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
	stub := purgeStub{requests: make(chan sessionpurge.Request, 4), reply: sessionpurge.Reply{
		Session:  "lamp",
		Targets:  []sessionpurge.Target{{Kind: "journal", Path: "/state/sessions/x", Files: 4, Bytes: 5 * 1024 * 1024}, {Kind: "record", Path: "manager database", Files: 1}},
		Retained: []string{"project workspace files the agent wrote"},
	}}
	resource.RegisterProjectServiceServer(server, stub)
	go server.Serve(listener)
	defer server.Stop()

	// Without a confirmation purge must not reach the daemon at all.
	got := xlitest.Run(t, newRoot(root), "session", "purge", "lamp")
	if got.Err == nil || !strings.Contains(got.Err.Error(), "--yes") {
		t.Fatal("purge ran unconfirmed", got.Err)
	}
	if len(stub.requests) != 0 {
		t.Fatal("an unconfirmed purge still called the daemon")
	}

	got = xlitest.Run(t, newRoot(root), "session", "purge", "--dry-run", "lamp")
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	if q := <-stub.requests; q.Session != "lamp" || !q.DryRun {
		t.Fatal("wrong request", q)
	}
	if !strings.Contains(got.Stdout, "Would delete for lamp") || !strings.Contains(got.Stdout, "5.0 MiB") {
		t.Fatal("dry run did not report what is at stake", got.Stdout)
	}
	if strings.Contains(got.Stdout, "requires backups") {
		t.Fatal("a dry run claimed data was deleted", got.Stdout)
	}

	got = xlitest.Run(t, newRoot(root), "session", "purge", "--yes", "lamp")
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	if q := <-stub.requests; q.Session != "lamp" || q.DryRun {
		t.Fatal("wrong request", q)
	}
	for _, want := range []string{"Deleted for lamp", "journal", "record", "Total: 5.0 MiB", "Retained: project workspace files", "requires backups"} {
		if !strings.Contains(got.Stdout, want) {
			t.Fatal("missing", want, "in", got.Stdout)
		}
	}

	got = xlitest.Run(t, newRoot(root), "session", "purge", "--yes", "--format", "json", "lamp")
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	<-stub.requests
	var reply sessionpurge.Reply
	if err = json.Unmarshal([]byte(got.Stdout), &reply); err != nil || len(reply.Targets) != 2 {
		t.Fatal("json output is not the reply", got.Stdout, err)
	}
}
