package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Compose delegates builds to buildx bake, and bake refuses to read the
// generated Dockerfile the devcontainer CLI points it at. Without the grant
// every Compose devcontainer -- including the built-in default -- fails before
// the build starts, so this is not a preference but the thing that makes
// provisioning work at all.
func TestProvisionEnvGrantsTheGeneratedDockerfile(t *testing.T) {
	env := provisionEnv("cxz-owner-project")
	want := map[string]string{
		"COMPOSE_PROJECT_NAME":        "cxz-owner-project",
		"BUILDX_BAKE_ENTITLEMENTS_FS": "0",
	}
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatal("not an environment assignment:", entry)
		}
		if expected, known := want[name]; !known || value != expected {
			t.Fatal(name, "is", value, "want", expected)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatal("missing from the provisioning environment:", want)
	}
}

// A failure that names a log file and nothing else sends every reader to the
// same place to find the one line that mattered. The line comes with the error.
func TestProvisionFailureCarriesTheOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provision.log")
	if got := provisionTail(path); !strings.Contains(got, "provision.log") {
		t.Fatal("a missing log should still point at itself:", got)
	}
	var log strings.Builder
	for i := range 200 {
		fmt.Fprintf(&log, "[timestamp] step %d\n", i)
	}
	log.WriteString(`additional privileges requested: pass "--allow=fs.read=/tmp/x" to grant requested privileges` + "\n\n")
	log.WriteString("Exit code 1\nError: Command failed: docker compose build\n")
	log.WriteString("    at Dp (/usr/local/lib/node_modules/@devcontainers/cli/dist/x.js:434:525)\n")
	log.WriteString("    at process.processTicksAndRejections (node:internal/process/task_queues:104:5)\n")
	if err := os.WriteFile(path, []byte(log.String()), 0600); err != nil {
		t.Fatal(err)
	}
	got := provisionTail(path)
	if !strings.Contains(got, "additional privileges requested") || !strings.Contains(got, "Exit code 1") {
		t.Fatal("the cause was not carried out of the log:", got)
	}
	// Stack frames name the CLI's internals, never the cause.
	if strings.Contains(got, "    at ") || strings.Contains(got, "node_modules") {
		t.Fatal("stack frames reached the error:", got)
	}
	if lines := strings.Count(got, "\n"); lines > 7 {
		t.Fatal("carried", lines, "lines; an error is not a log viewer")
	}
	// A long line is clipped rather than pasted whole into an RPC error.
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 4000)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := provisionTail(path); len(got) > 500 {
		t.Fatal("a single line brought", len(got), "bytes")
	}
}
