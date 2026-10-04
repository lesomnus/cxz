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

func TestWebSavedSettingsAndExplicitConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "web-installation.json"), []byte(`{"origin":"https://saved.example","tls_key":"saved.key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(t.TempDir(), "custom.json")
	os.WriteFile(custom, []byte(`{"origin":"https://custom.example","tls_key":"custom.key"}`), 0600)
	for _, tc := range []struct {
		args        []string
		origin, key string
	}{
		{[]string{"web"}, "https://saved.example", filepath.Join(root, "saved.key")},
		{[]string{"web", "--config", custom}, "https://custom.example", filepath.Join(filepath.Dir(custom), "custom.key")},
	} {
		cmd := newRoot(root)
		var cfg webconfig.Config
		cmd.Commands.Get("web").Handler = onRun(func(ctx context.Context, c *xli.Command) error {
			var err error
			cfg, err = readWebConfig(ctx, c)
			return err
		})
		result := xlitest.Run(t, cmd, tc.args...)
		if result.Err != nil || cfg.Origin != tc.origin || cfg.Key != tc.key {
			t.Fatalf("%+v %v", cfg, result.Err)
		}
	}
}
