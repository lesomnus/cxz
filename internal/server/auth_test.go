package server

import (
	"context"
	"os"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLaunchRejectsInvalidStoredAuth(t *testing.T) {
	for _, backend := range []string{"", accounts.BrokeredAccessToken, "unknown", accounts.ProjectLocalOAuth} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			s := &Server{root: root}
			_, err := s.launch(context.Background(), core.Session{ID: "session", ProjectID: "project", Account: "work", Kind: "claude", AuthBackend: backend, AuthBinding: "wrong-binding"})
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatal("invalid stored auth was not rejected", err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("launch performed side effects before validating binding", err)
			}
		})
	}
}
