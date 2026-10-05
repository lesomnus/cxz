//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/webconfig"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/xlitest"
)

// webConfigProbe replaces a subcommand's handler with the configuration step
// alone, so the resolution rules can be read without Docker.
func webConfigProbe(t *testing.T, root, sub string, read func(context.Context, *xli.Command) (webconfig.Config, error), args ...string) (webconfig.Config, xlitest.Result) {
	t.Helper()
	cmd := newRoot(root)
	var got webconfig.Config
	cmd.Commands.Get("web").Commands.Get(sub).Handler = onRun(func(ctx context.Context, c *xli.Command) error {
		var err error
		got, err = read(ctx, c)
		return err
	})
	return got, xlitest.Run(t, cmd, append([]string{"web", sub}, args...)...)
}

func TestWebSettingsComeFromFileThenFlags(t *testing.T) {
	for _, sub := range []string{"up", "serve"} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "web.json"), []byte(`{"listen":"127.0.0.1:7443","origin":"https://file.example","tls_cert":"cert.pem"}`), 0600)
		got, result := webConfigProbe(t, root, sub, readWebConfigValue, "--origin", "https://flag.example", "--listen", "0.0.0.0:7351")
		if result.Err != nil {
			t.Fatal(sub, result.Err)
		}
		if got.Origin != "https://flag.example" || got.Listen != "0.0.0.0:7351" || got.Certificate != filepath.Join(root, "cert.pem") {
			t.Fatalf("%s: %+v", sub, got)
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
		{nil, "https://saved.example", filepath.Join(root, "saved.key")},
		{[]string{"--config", custom}, "https://custom.example", filepath.Join(filepath.Dir(custom), "custom.key")},
	} {
		got, result := webConfigProbe(t, root, "serve", readWebConfigValue, tc.args...)
		if result.Err != nil || got.Origin != tc.origin || got.Key != tc.key {
			t.Fatalf("%+v %v", got, result.Err)
		}
	}
}

// With nothing configured, `web up` writes what a desktop needs instead of
// reporting a usage error: a token, and a loopback origin that asks for no
// certificate. The files it writes are private, and it writes them once.
func TestWebUpGeneratesALoopbackConfiguration(t *testing.T) {
	root := t.TempDir()
	got, result := webConfigProbe(t, root, "up", webUpConfig)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if got.Origin != "http://127.0.0.1:7350" || got.Listen != "127.0.0.1:7350" {
		t.Fatalf("%+v", got)
	}
	if got.Certificate != "" || got.Key != "" {
		t.Fatal("a loopback default asked for a certificate:", got.Certificate, got.Key)
	}
	if _, err := got.Runtime(); err != nil {
		t.Fatal("the generated configuration does not serve:", err)
	}
	for _, name := range []string{"web.json", "web-token"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s is %v", name, info.Mode().Perm())
		}
	}
	if !strings.Contains(result.Stderr, "web-token") || !strings.Contains(result.Stderr, "web.json") {
		t.Fatal("generation was silent:", result.Stderr)
	}
	// The token may already be in a browser, and the file may already have been
	// edited, so a second run adopts both rather than replacing them.
	token, err := os.ReadFile(filepath.Join(root, "web-token"))
	if err != nil {
		t.Fatal(err)
	}
	again, result := webConfigProbe(t, root, "up", webUpConfig)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if strings.Contains(result.Stderr, "Generated") {
		t.Fatal("a second run rewrote the configuration:", result.Stderr)
	}
	if again.TokenFile != filepath.Join(root, "web-token") {
		t.Fatalf("%+v", again)
	}
	if later, _ := os.ReadFile(filepath.Join(root, "web-token")); string(later) != string(token) {
		t.Fatal("the token changed under a signed-in browser")
	}
}

// A named configuration is a claim that it exists. Generating one instead would
// hide a typo in --config behind a gateway nobody asked for.
func TestWebUpDoesNotGenerateForANamedConfig(t *testing.T) {
	root := t.TempDir()
	_, result := webConfigProbe(t, root, "up", webUpConfig, "--config", filepath.Join(root, "absent.json"))
	if result.Err == nil {
		t.Fatal("a missing named configuration was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "web.json")); err == nil {
		t.Fatal("a named configuration generated the default one")
	}
}

// Plaintext is allowed because a loopback address is not a network. Asking for
// it on an address that is one is refused where the listener is bound.
func TestWebServeRefusesPlaintextOnANetworkAddress(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "web.json"), []byte(`{"listen":"0.0.0.0:7350","origin":"http://127.0.0.1:7350","access_token_file":"token"}`), 0600)
	os.WriteFile(filepath.Join(root, "token"), []byte(strings.Repeat("a", 64)), 0600)
	cmd := newRoot(root)
	result := xlitest.Run(t, cmd, "web", "serve")
	if result.Err == nil || !strings.Contains(result.Err.Error(), "loopback") {
		t.Fatal("plaintext on every interface was accepted:", result.Err)
	}
}

// Nothing configured anywhere is an absent configuration, not a malformed one.
func TestWebServeNamesTheCommandThatWritesAConfiguration(t *testing.T) {
	result := xlitest.Run(t, newRoot(t.TempDir()), "web", "serve")
	if result.Err == nil || !strings.Contains(result.Err.Error(), "cxz web up") {
		t.Fatal(result.Err)
	}
}

func TestWebStatusReadsConfigurationWithoutAManager(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "web.json"), []byte(`{"listen":"127.0.0.1:7350","origin":"http://127.0.0.1:7350","access_token_file":"web-token"}`), 0600)
	result := xlitest.Run(t, newRoot(root), "web", "status", "--format", "json")
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var state struct {
		ConfigPath   string `json:"config_path"`
		Origin       string `json:"origin"`
		TLS          bool   `json:"tls"`
		TokenPresent bool   `json:"token_present"`
		State        string `json:"state"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &state); err != nil {
		t.Fatal(err, result.Stdout)
	}
	if state.ConfigPath != filepath.Join(root, "web.json") || state.Origin != "http://127.0.0.1:7350" || state.TLS {
		t.Fatalf("%+v", state)
	}
	if state.TokenPresent {
		t.Fatal("a missing token was reported as present")
	}
	if !strings.Contains(state.State, "not installed") {
		t.Fatal("status did not say the Manager is absent:", state.State)
	}
	table := xlitest.Run(t, newRoot(root), "web", "status")
	if table.Err != nil || !strings.Contains(table.Stdout, "http (loopback)") || !strings.Contains(table.Stdout, "(missing)") {
		t.Fatalf("%q %v", table.Stdout, table.Err)
	}
}

func readWebConfigValue(ctx context.Context, c *xli.Command) (webconfig.Config, error) {
	cfg, _, err := readWebConfig(ctx, c)
	return cfg, err
}
