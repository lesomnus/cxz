package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/workspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDevcontainerSettingsPersistAndClear(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := &Server{manager: &workspace.Manager{Root: root}}
	request := &api.DevcontainerInput{Spec: []byte(`{"compose":{"services":{"${DEVCONTAINER_SERVICE}":{"volumes":["/host:/workspaces"]}}}}`)}
	if _, err := s.Devcontainer(ctx, request); err != nil {
		t.Fatal(err)
	}
	saved, err := projectconfig.Load(root)
	if err != nil || len(saved.Compose) == 0 {
		t.Fatal(saved, err)
	}
	for _, invalid := range []string{`invalid`, `{"compose":"host-file.yaml"}`, `{"compose":{"services":[]}}`} {
		if _, err := s.Devcontainer(ctx, &api.DevcontainerInput{Spec: []byte(invalid)}); err == nil {
			t.Fatal("invalid settings accepted", invalid)
		}
	}
	got, err := projectconfig.Load(root)
	if err != nil || string(saved.Compose) != string(got.Compose) {
		t.Fatal("invalid settings replaced saved snapshot", err)
	}
	// A fresh server instance needs only manager state, not the original host file.
	s = &Server{manager: &workspace.Manager{Root: root}}
	data, _ := json.Marshal(projectconfig.Spec{})
	if _, err := s.Devcontainer(ctx, &api.DevcontainerInput{Spec: data}); err != nil {
		t.Fatal(err)
	}
	got, err = projectconfig.Load(root)
	if err != nil || len(got.Compose) != 0 {
		t.Fatal("clear did not persist", got, err)
	}
	if _, err := (&Server{}).Devcontainer(ctx, request); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("project runtime accepted host settings", err)
	}
}
