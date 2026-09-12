package accounts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRefreshDelegatesToManagedCodex(t *testing.T) {
	root := t.TempDir()
	seedCentral(t, root, "work", "workspace", "old")
	binary := filepath.Join(root, "codex")
	script := `#!/bin/sh
set -eu
test "$*" = '-c cli_auth_credentials_store="file" -c model_provider="openai" app-server --listen stdio://'
test -z "${OPENAI_API_KEY:-}"
IFS= read -r line
printf '%s\n' '{"id":"init","result":{}}'
IFS= read -r line
IFS= read -r line
case "$line" in *'"refreshToken":true'*) ;; *) exit 4;; esac
printf '%s\n' '{"id":"refresh","result":{"account":{"type":"chatgpt"}}}'
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "synthetic-inherited")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := RefreshManaged(ctx, root, "work", binary); err != nil {
		t.Fatal(err)
	}
	script = strings.ReplaceAll(script, `{"type":"chatgpt"}`, `null`)
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := RefreshManaged(ctx, root, "work", binary); err == nil {
		t.Fatal("logged-out response accepted")
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec sleep 20\n"), 0700); err != nil {
		t.Fatal(err)
	}
	timeout, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	start := time.Now()
	if err := RefreshManaged(timeout, root, "work", binary); err == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("refresh did not honor cancellation")
	}
}
