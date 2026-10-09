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

// A purge is a session operation, so the stub serves it on the session service.
type purgeStub struct {
	resource.UnimplementedSessionServiceServer
	requests chan sessionpurge.Request
	reply    sessionpurge.Reply
}

func (s purgeStub) Purge(_ context.Context, r *resource.SessionPurgeRequest) (*resource.SessionPurgeReply, error) {
	handle := r.GetRef().GetAlias()
	if handle == "" {
		handle = r.GetRef().GetRuntimeId()
	}
	s.requests <- sessionpurge.Request{Session: handle, DryRun: r.GetDryRun()}
	out := resource.SessionPurgeReply_builder{
		Ref: r.GetRef(), DryRun: ptr(r.GetDryRun()), Retained: s.reply.Retained,
	}
	for _, t := range s.reply.Targets {
		out.Targets = append(out.Targets, resource.SessionPurgeTarget_builder{
			Kind: ptr(t.Kind), Path: ptr(t.Path), Files: ptr(int32(t.Files)), Bytes: ptr(t.Bytes),
		}.Build())
	}
	return out.Build(), nil
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
	resource.RegisterSessionServiceServer(server, stub)
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
