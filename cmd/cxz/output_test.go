//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/xlitest"
)

func TestResourceCommandTree(t *testing.T) {
	for _, name := range []string{"projects", "ls", "new", "get", "send", "reply", "stop", "resume", "interrupt", "events", "serve", "update", "rollback", "logs"} {
		got := xlitest.Run(t, newRoot("unused"), name)
		if !errors.Is(got.Err, xli.ErrUnknownCmd) {
			t.Fatalf("old command %s: %v", name, got.Err)
		}
	}
	for _, group := range []string{"project", "session", "account", "manager", "config", "backend", "binding"} {
		got := xlitest.Run(t, newRoot("unused"), group)
		if got.Err != nil || !strings.Contains(got.Stdout, "Commands:") {
			t.Fatalf("%s: %+v", group, got)
		}
	}
	for _, input := range []string{"session ls --format ", "project ls --format ", "--format "} {
		got := xlitest.Complete(t, newRoot("unused"), input)
		if got.Err != nil || !got.Has("table") || !got.Has("json") {
			t.Fatalf("%s: %+v", input, got)
		}
	}
}

func TestOutputFormats(t *testing.T) {
	fixture := &api.ProjectList{Projects: []*api.Project{{Id: "id", Alias: "work", Name: "한글 프로젝트", Workspace: "/workspace", State: "running", Error: "diagnostic-only-in-json"}}}
	for _, tc := range []struct {
		args []string
		json bool
	}{
		{[]string{"project", "ls"}, false},
		{[]string{"project", "ls", "--format", "json"}, true},
		{[]string{"--format", "json", "project", "ls"}, true},
		{[]string{"--format", "json", "project", "ls", "--format", "table"}, false},
	} {
		root := newRoot("unused")
		root.Commands.Get("project").Commands.Get("ls").Handler = onRun(func(_ context.Context, c *xli.Command) error { return writeOutput(c, fixture) })
		got := xlitest.Run(t, root, tc.args...)
		if got.Err != nil {
			t.Fatal(got.Err)
		}
		if tc.json {
			var v api.ProjectList
			if json.Unmarshal([]byte(got.Stdout), &v) != nil || len(v.Projects) != 1 || v.Projects[0].Error != fixture.Projects[0].Error {
				t.Fatal("JSON fields lost", got.Stdout)
			}
		} else if !strings.Contains(got.Stdout, "ALIAS") || !strings.Contains(got.Stdout, "한글 프로젝트") || strings.HasPrefix(got.Stdout, "{") {
			t.Fatal(got.Stdout)
		}
	}
	for _, format := range []string{"yaml", "", "JSON"} {
		got := xlitest.Run(t, newRoot("/does-not-exist"), "project", "ls", "--format", format)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "format must be") {
			t.Fatal(got)
		}
	}
}

func TestEmptyAndResourceOutput(t *testing.T) {
	for _, v := range []any{&api.ProjectList{}, &api.SessionList{}} {
		root := newRoot("unused")
		root.Commands.Get("session").Commands.Get("ls").Handler = onRun(func(_ context.Context, c *xli.Command) error { return writeOutput(c, v) })
		got := xlitest.Run(t, root, "session", "ls")
		if got.Err != nil || got.Stdout != "No results.\n" {
			t.Fatal(got)
		}
	}
	for _, format := range []string{"table", "json"} {
		root := newRoot("unused")
		root.Commands.Get("account").Commands.Get("ls").Handler = onRun(func(_ context.Context, c *xli.Command) error {
			return writeResource(c, resource.AccountListResponse_builder{Items: []*resource.Account{resource.Account_builder{Alias: "work", Name: "회사", Agent: "claude", AuthBackend: "project-local-oauth"}.Build()}}.Build())
		})
		got := xlitest.Run(t, root, "account", "ls", "--format", format)
		if got.Err != nil || !strings.Contains(got.Stdout, "project-local-oauth") {
			t.Fatal(got)
		}
		if format == "json" && (!strings.Contains(got.Stdout, `"authBackend"`) || !strings.Contains(got.Stdout, `"items"`)) {
			t.Fatal("protobuf JSON contract changed")
		}
	}
}

func TestTableCellsAndWriteErrors(t *testing.T) {
	cell := tableCell("hello\n\t\x1b[31m" + strings.Repeat("한", 100))
	if strings.ContainsAny(cell, "\n\t\x1b") || ansi.StringWidth(cell) > 72 {
		t.Fatal("unsafe or unbounded cell")
	}
	var out strings.Builder
	if err := writeTable(&out, []string{"name", "state"}, [][]string{{"한글", "idle"}, {"abcd", "idle"}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out.String(), "\n")
	if strings.Index(lines[1], "idle")-len("한글") != strings.Index(lines[2], "idle")-len("abcd") {
		t.Fatal("unicode column padding incorrect", out.String())
	}
	if writeTable(failedWriter{}, []string{"id"}, nil) == nil {
		t.Fatal("write error ignored")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestEventJSONLines(t *testing.T) {
	root := newRoot("unused")
	root.Commands.Get("session").Commands.Get("events").Handler = onRun(func(_ context.Context, c *xli.Command) error {
		for _, seq := range []uint64{9007199254740993, 9007199254740994} {
			if err := writeOutput(c, &api.Event{Seq: seq, Kind: "assistant", Text: "first\nsecond"}); err != nil {
				return err
			}
		}
		return nil
	})
	got := xlitest.Run(t, root, "session", "events", "--format", "json", "session")
	lines := strings.Split(strings.TrimSpace(got.Stdout), "\n")
	if got.Err != nil || len(lines) != 2 {
		t.Fatal(got)
	}
	for i, line := range lines {
		var event api.Event
		if json.Unmarshal([]byte(line), &event) != nil || event.Seq != 9007199254740993+uint64(i) || event.Text != "first\nsecond" {
			t.Fatal("lossy JSONL", line)
		}
	}
}

func TestAccountStatusOutput(t *testing.T) {
	root := newRoot("unused")
	root.Commands.Get("account").Commands.Get("status").Handler = onRun(func(ctx context.Context, c *xli.Command) error {
		return runAccountProcess(exec.CommandContext(ctx, "sh", "-c", "printf legacy-status"), c, resource.Account_builder{Alias: "work", Agent: "claude", AuthBackend: "project-local-oauth"}.Build(), "status")
	})
	got := xlitest.Run(t, root, "account", "status", "--format", "json", "work")
	if got.Err != nil || strings.Contains(got.Stdout, "legacy-status") || !strings.Contains(got.Stdout, `"credential_present":true`) || !strings.Contains(got.Stdout, `"vendor_verified":false`) {
		t.Fatal(got)
	}
}
