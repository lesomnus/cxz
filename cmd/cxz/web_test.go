//go:build !windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/webconfig"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/xlitest"
)

func TestWebSettingsAndInstallRoute(t *testing.T) {
	for _, installed := range []bool{false, true} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "web.json"), []byte(`{"listen":"127.0.0.1:7443","origin":"https://file.example","tls_cert":"cert.pem"}`), 0600)
		cmd := newRoot(root)
		c := cmd.Commands.Get("web")
		args := []string{"web"}
		if installed {
			c = cmd.Commands.Get("install").Commands.Get("web")
			args = []string{"install", "web"}
		}
		var got webconfig.Config
		c.Handler = onRun(func(ctx context.Context, c *xli.Command) error {
			var err error
			got, err = readWebConfig(ctx, c)
			return err
		})
		result := xlitest.Run(t, cmd, append(args, "--origin", "https://flag.example", "--listen", "0.0.0.0:7351")...)
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		if got.Origin != "https://flag.example" || got.Listen != "0.0.0.0:7351" || got.Certificate != filepath.Join(root, "cert.pem") {
			t.Fatalf("%+v", got)
		}
	}
}
