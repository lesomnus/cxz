package telemetrystack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacementPreflightPreservesExistingServices(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "pull failure", true: "foreign owner"}[foreign], func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			t.Setenv("CXZ_TELEMETRY_TEST_LOG", log)
			owner := "owned-installation"
			existingOwner := owner
			if foreign {
				existingOwner = "another-installation"
			}
			// Inspect finds an existing service; replacement image lookup/pull fail.
			script := `#!/bin/sh
printf '%s\n' "$1" >> "$CXZ_TELEMETRY_TEST_LOG"
if [ "$1" = inspect ]; then
  printf '%s\n' '[{"Id":"existing","Config":{"Image":"old","Labels":{"cxz.owner":"` + existingOwner + `","cxz.project":"telemetry"}},"State":{"Running":true}}]'
  exit 0
fi
exit 1
`
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			stack := Stack{Owner: owner}
			if err := stack.ensureContainer(t.Context(), "service", "new", "new configuration", nil); err == nil {
				t.Fatal("unsafe replacement succeeded")
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(calls), "rm") || strings.Contains(string(calls), "stop") || strings.Contains(string(calls), "run") {
				t.Fatal("preflight mutated existing service", string(calls))
			}
			if foreign && string(calls) != "inspect\n" {
				t.Fatal("foreign ownership was checked after image actions", string(calls))
			}
		})
	}
}

func TestExternalStack(t *testing.T) {
	s := Stack{Root: t.TempDir(), Owner: strings.Repeat("o", 24), External: true, DBHost: "cxz-telemetry-db", Password: strings.Repeat("p", 32), Listen: "0.0.0.0:4317"}
	config := s.CollectorConfig(s.Password)
	for _, want := range []string{"endpoint: 0.0.0.0:4317\n        transport: tcp", "endpoint: tcp://cxz-telemetry-db:9000?"} {
		if !strings.Contains(config, want) {
			t.Fatalf("collector config lacks %q:\n%s", want, config)
		}
	}
	if s.Endpoint() != "http://cxz-telemetry-db:8123" {
		t.Fatal(s.Endpoint())
	}
	if err := s.Prepare(); err != nil {
		t.Fatal(err)
	}
	if client, err := s.Client(); err != nil || client.Password != s.Password {
		t.Fatal("the deployment's credential was not used", err)
	}
	weak := s
	weak.Password = "short"
	if weak.Prepare() == nil {
		t.Fatal("a short database password was accepted")
	}
	docker := Stack{Owner: strings.Repeat("o", 24)}
	if c := docker.CollectorConfig("x"); !strings.Contains(c, "endpoint: /var/run/cxz-telemetry/collector.sock\n        transport: unix") || !strings.Contains(c, "tcp://cxz-oooooooooooo-telemetry-db:9000?") {
		t.Fatal("the Docker collector configuration changed")
	}
}

// Installations that share one server each have an account and a database.
func TestSharedDatabase(t *testing.T) {
	s := Stack{Root: t.TempDir(), Owner: strings.Repeat("o", 24), External: true, Password: strings.Repeat("p", 32), Listen: "0.0.0.0:4317",
		DBHTTP: "https://clickhouse.example:8443", DBNative: "tcp://clickhouse.example:9440?secure=true", DBUser: "cxz_alice", DBName: "cxz_alice"}
	config := s.CollectorConfig(s.Password)
	for _, want := range []string{"endpoint: tcp://clickhouse.example:9440?secure=true&dial_timeout=5s", "username: cxz_alice", "database: cxz_alice"} {
		if !strings.Contains(config, want) {
			t.Fatalf("collector config lacks %q:\n%s", want, config)
		}
	}
	if err := s.Prepare(); err != nil {
		t.Fatal(err)
	}
	client, err := s.Client()
	if err != nil || client.Endpoint != "https://clickhouse.example:8443" || client.User != "cxz_alice" || client.Database != "cxz_alice" {
		t.Fatal(client, err)
	}
	for _, bad := range []string{"cxz-alice", "1cxz", "x; DROP"} {
		b := s
		b.DBName = bad
		if b.Prepare() == nil {
			t.Errorf("database name %q accepted", bad)
		}
	}
	partial := s
	partial.DBNative = ""
	if partial.Prepare() == nil {
		t.Fatal("an HTTP endpoint without a native one, and no host, was accepted")
	}
}
