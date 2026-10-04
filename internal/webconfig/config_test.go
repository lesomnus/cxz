package webconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.json")
	cfg, err := Load(path, true)
	if err != nil || cfg.Listen != "127.0.0.1:7350" {
		t.Fatalf("%+v %v", cfg, err)
	}
	if _, err = Load(path, false); err == nil {
		t.Fatal("missing explicit config accepted")
	}
	if err = os.WriteFile(path, []byte(`{"origin":"https://test:7350","tls_cert":"cert.pem","tls_key":"key.pem","access_token_file":"token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Certificate != filepath.Join(dir, "cert.pem") || cfg.TokenFile != filepath.Join(dir, "token") || cfg.Listen != "127.0.0.1:7350" {
		t.Fatalf("%+v", cfg)
	}
	for _, bad := range []string{`{"tls_certt":"oops"}`, `{} {}`, `{"listen":5}`} {
		os.WriteFile(path, []byte(bad), 0600)
		if _, err = Load(path, false); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
