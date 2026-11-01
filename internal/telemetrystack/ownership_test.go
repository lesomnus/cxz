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
