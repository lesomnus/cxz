package settings

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/transport"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConnectionsRoundTrip(t *testing.T) {
	raw := []byte(`{"connections":{"default":"work","work":{"target":"ssh://work"},"home":{"target":"local://"},"dev":{"target":"unix://${STATE}/run/daemon.sock"},"vpn":{"target":"tcp://home:7349","token_file":"tokens/home"}}}`)
	cfg, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connections.DefaultName() != "work" {
		t.Fatal(cfg.Connections)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(b)
	if err != nil || !reflect.DeepEqual(cfg, got) {
		t.Fatal(string(b), got, err)
	}
	root := t.TempDir()
	c := cfg.Connections.Entries["dev"].Resolve(root)
	if c.Target != "unix://"+filepath.ToSlash(root)+"/run/daemon.sock" {
		t.Fatal(c)
	}
	if c := cfg.Connections.Entries["vpn"].Resolve(root); c.TokenFile != filepath.Join(root, "tokens/home") {
		t.Fatal(c)
	}
}
func TestInvalidConnections(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"default":"missing","work":{"target":"ssh://work"}}`,
		`{"work":{"target":"ssh://work","typo":true}}`,
		`{"work::bad":{"target":"ssh://work"}}`,
		`{"work":{"target":"tcp://host:7349"}}`,
		`{"work":{"target":"ssh://work","token_file":"unused"}}`,
		`{"work":{"target":"unix://${UNKNOWN}/socket"}}`,
		`{"work":{"target":"unix://host/socket"}}`,
		`{"work":{"target":"local://host"}}`,
	} {
		if _, err := Parse([]byte(`{"connections":` + raw + `}`)); err == nil {
			t.Error("accepted", raw)
		}
	}
}

func TestConnectionEscapesStatePath(t *testing.T) {
	entry := Connection{Target: "unix://${STATE}/run/daemon.sock"}.Resolve("/tmp/a#b?100%")
	parsed, err := transport.ParseEndpoint(entry.Target)
	if err != nil || parsed.Address != "/tmp/a#b?100%/run/daemon.sock" {
		t.Fatal(entry, parsed, err)
	}
}
