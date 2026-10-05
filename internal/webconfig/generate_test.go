package webconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The generated configuration has to be servable as written, with no second
// step: that is the whole point of generating it.
func TestGeneratedConfigurationServesAsWritten(t *testing.T) {
	root := t.TempDir()
	cfg, created, err := Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created %v", created)
	}
	runtime, err := cfg.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Certificate != "" || runtime.Key != "" {
		t.Fatal("a loopback gateway was given TLS files")
	}
	if len(runtime.Token) < 64 {
		t.Fatal("token is shorter than the 32 bytes it reads back as hex:", len(runtime.Token))
	}

	// Reading the written file back must produce the same thing, including the
	// token path, which is stored relative so a moved state directory survives.
	b, err := os.ReadFile(DefaultPath(root))
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]any
	if err = json.Unmarshal(b, &written); err != nil {
		t.Fatal(err)
	}
	if written["access_token_file"] != "web-token" {
		t.Fatalf("%v", written)
	}
	if _, ok := written["tls_cert"]; ok {
		t.Fatalf("an unused certificate path was written: %v", written)
	}
	reloaded, err := Load(DefaultPath(root), false)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded != cfg {
		t.Fatalf("%+v != %+v", reloaded, cfg)
	}
}

func TestGenerateKeepsWhatItFinds(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(TokenPath(root), []byte(strings.Repeat("b", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, created, err := Generate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0] != DefaultPath(root) {
		t.Fatalf("created %v", created)
	}
	runtime, err := cfg.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Token != strings.Repeat("b", 64) {
		t.Fatal("an existing token was replaced")
	}
	if _, err = os.Stat(filepath.Join(root, "web.json")); err != nil {
		t.Fatal(err)
	}
}
