package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/accounts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

func TestProjectLoginRequiredDetails(t *testing.T) {
	root := t.TempDir()
	_, missing := accounts.Credential(root, "main", "claude")
	var required *accounts.LoginRequired
	if !errors.As(missing, &required) {
		t.Fatal(missing)
	}
	for _, backend := range []string{accounts.ProjectLocalOAuth, accounts.BrokeredAccessToken} {
		s := status.Convert(authCheckError(missing, backend))
		if backend == accounts.BrokeredAccessToken {
			if len(s.Details()) != 0 {
				t.Fatal("central auth offered project login")
			}
			continue
		}
		if len(s.Details()) != 1 {
			t.Fatal(s)
		}
		d := s.Details()[0].(*errdetails.ErrorInfo)
		if d.Domain != "cxz.auth" || d.Reason != "PROJECT_LOGIN_REQUIRED" || d.Metadata["account"] != "main" {
			t.Fatal(d)
		}
	}
	if err := accounts.Prepare(root, "main", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accounts.Config(root, "main"), ".credentials.json"), []byte(`invalid`), 0600); err != nil {
		t.Fatal(err)
	}
	_, corrupt := accounts.Credential(root, "main", "claude")
	if corrupt == nil || len(status.Convert(authCheckError(corrupt, accounts.ProjectLocalOAuth)).Details()) != 0 {
		t.Fatal("corrupt credentials offered automatic overwrite")
	}
}

func TestClearedClaudeCredentialsRequestSessionLogin(t *testing.T) {
	for _, credential := range []string{`{}`, `{"claudeAiOauth":null}`, `{"claudeAiOauth":{"accessToken":""}}`} {
		t.Run(credential, func(t *testing.T) {
			root := t.TempDir()
			if err := accounts.Prepare(root, "work1", "claude"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(accounts.Config(root, "work1"), ".credentials.json"), []byte(credential), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := accounts.Credential(root, "work1", "claude")
			st := status.Convert(authCheckError(err, accounts.ProjectLocalOAuth, "original-creation"))
			if len(st.Details()) != 1 {
				t.Fatal(st)
			}
			info := st.Details()[0].(*errdetails.ErrorInfo)
			if info.Metadata["account"] != "work1" || info.Metadata["session_key"] != "original-creation" {
				t.Fatal(info)
			}
			wrapped := status.Convert(fmt.Errorf("resume: project runtime Resume RPC: %w", st.Err()))
			if len(wrapped.Details()) != 1 {
				t.Fatal("resume dropped login binding", wrapped)
			}
		})
	}
}
