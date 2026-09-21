//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/settings"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type devcontainerSink struct {
	api.SessionsClient
	spec  projectconfig.Spec
	err   error
	calls int
}

func (s *devcontainerSink) Devcontainer(_ context.Context, r *api.DevcontainerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	s.calls++
	s.spec = projectconfig.Spec{}
	if err := json.Unmarshal(r.Spec, &s.spec); err != nil {
		return nil, err
	}
	return &api.Receipt{Status: "saved"}, s.err
}

func TestPublishDevcontainerSnapshotsAndCompatibility(t *testing.T) {
	root := t.TempDir()
	sink := &devcontainerSink{}
	cfg := settings.Config{Devcontainer: projectconfig.Config{Compose: json.RawMessage(`"override.yaml"`)}}
	for _, source := range []string{"/first", "/edited"} {
		if err := os.WriteFile(filepath.Join(root, "override.yaml"), []byte("services:\n  dev:\n    volumes:\n      - "+source+":/workspaces\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := syncDevcontainer(context.Background(), sink, root, cfg); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(sink.spec.Compose), source) {
			t.Fatal("published a path or stale file", sink.spec)
		}
	}
	if _, err := syncDevcontainer(context.Background(), sink, root, settings.Config{}); err != nil || len(sink.spec.Compose) != 0 {
		t.Fatal("removed settings not published", sink.spec, err)
	}
	sink.err = status.Error(codes.Unimplemented, "older manager")
	if _, err := syncDevcontainer(context.Background(), sink, root, cfg); err == nil || !strings.Contains(err.Error(), "cxz install --recreate") {
		t.Fatal("override silently lost on older manager", err)
	}
	if _, err := syncDevcontainer(context.Background(), sink, root, settings.Config{}); err != nil {
		t.Fatal("empty settings require upgrade", err)
	}
	if err := os.Remove(filepath.Join(root, "override.yaml")); err != nil {
		t.Fatal(err)
	}
	before := sink.calls
	if _, err := syncDevcontainer(context.Background(), sink, root, cfg); err == nil || sink.calls != before {
		t.Fatal("missing file changed manager settings", err)
	}
}

func TestEditRejectsUnreadableDevcontainerOverride(t *testing.T) {
	root := t.TempDir()
	if err := settings.Save(root, settings.Config{ClaudeModel: "keep"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(settings.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	changed, _, err := editSettings(root, func(path string) error {
		return os.WriteFile(path, []byte(`{"devcontainer":{"compose":"missing.yaml"}}`), 0600)
	})
	if err == nil || changed || !strings.Contains(err.Error(), "draft kept") {
		t.Fatal(changed, err)
	}
	after, err := os.ReadFile(settings.Path(root))
	if err != nil || string(after) != string(before) {
		t.Fatal("invalid override replaced saved settings", err)
	}
}
