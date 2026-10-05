package installer

import (
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/transport"
)

// What the relay is given decides what it can do when it is reached, so the
// arguments are worth asserting without Docker: the manager's socket and its own
// leaf, read-only, and nothing that would let it issue a credential or write.
func TestRelayContainerIsolation(t *testing.T) {
	v := transport.Installation{Owner: "fixture", Container: "manager", Image: "fixture:new", StateVolume: "fixture-state"}
	args, err := relayArgs(v, RelayConfig{Listen: "0.0.0.0:7349"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--name manager-remote", "--restart unless-stopped", "cxz.role=remote", "--read-only", "--cap-drop=ALL",
		"--security-opt=no-new-privileges", "--publish 0.0.0.0:7349:7349",
		"volume-subpath=run,readonly", "volume-subpath=pki/server,readonly",
		"--state /var/lib/cxz -x _expose-serve --listen 0.0.0.0:7349",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
	for _, bad := range []string{"docker.sock", "target=/var/lib/cxz,", "volume-subpath=pki,", "DAC_OVERRIDE"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("unsafe argument %s: %s", bad, joined)
		}
	}
	// A published port is how this is reached, so the address has to be one
	// Docker can publish: a fixed port on an IP, or every interface.
	if args, err = relayArgs(v, RelayConfig{Listen: "127.0.0.1:7443"}); err != nil || !strings.Contains(strings.Join(args, " "), "--publish 127.0.0.1:7443:7349") {
		t.Fatalf("%v %v", args, err)
	}
	if args, err = relayArgs(v, RelayConfig{Listen: "[::1]:7443"}); err != nil || !strings.Contains(strings.Join(args, " "), "[::1]:7443:7349") {
		t.Fatalf("%v %v", args, err)
	}
	for _, bad := range []string{"", ":0", ":65536", "localhost:7349", "7349", "0.0.0.0"} {
		if _, err = relayArgs(v, RelayConfig{Listen: bad}); err == nil {
			t.Fatalf("accepted listen %q", bad)
		}
	}
}
