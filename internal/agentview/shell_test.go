package agentview

import (
	"encoding/json"
	"testing"
)

// A wrapper spends the row's width before the script starts and makes the script
// one quoted string to the highlighter. Unwrap only when the trailing operand
// really is the whole script, and leave anything else exactly as it came.
func TestUnwrapShell(t *testing.T) {
	for _, tc := range []struct{ command, shell, script string }{
		// The shape Codex uses.
		{`/usr/bin/zsh -lc "rg -n 'x' a.go; git log -1"`, "zsh", `rg -n 'x' a.go; git log -1`},
		{`bash -c 'make test'`, "bash", "make test"},
		{`/bin/sh -c "echo hi"`, "sh", "echo hi"},
		{`bash -o pipefail -c 'a | b'`, "bash", "a | b"},
		{`bash --norc -c 'a'`, "bash", "a"},

		// Not a wrapper: nothing to split.
		{`go test ./...`, "", `go test ./...`},
		{`zsh script.sh`, "", `zsh script.sh`}, // A file, not a script string.
		{`bash --no-rcs script.sh`, "", `bash --no-rcs script.sh`},
		{`bash -c 'echo $0' name`, "", `bash -c 'echo $0' name`}, // Operand is $0.
		{`python -c 'print(1)'`, "", `python -c 'print(1)'`},     // Not a shell.
		{`bash -c ''`, "", `bash -c ''`},
		{`bash -c`, "", `bash -c`},
		{`zsh -lc "unbalanced`, "", `zsh -lc "unbalanced`},
	} {
		shell, script := unwrapShell(tc.command)
		if shell != tc.shell || script != tc.script {
			t.Fatalf("unwrapShell(%q) = %q, %q; want %q, %q", tc.command, shell, script, tc.shell, tc.script)
		}
	}
}

func TestToolViewNamesTheShell(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		var raw []byte
		name := "Bash"
		if provider == "codex" {
			name = "item/commandExecution"
			raw, _ = json.Marshal(map[string]any{"item": map[string]any{"type": "commandExecution", "command": `/usr/bin/zsh -lc "git status"`}})
		} else {
			raw, _ = json.Marshal(map[string]any{"tool_name": name, "command": `/usr/bin/zsh -lc "git status"`})
		}
		a, ok := ToolView(provider, name, raw)
		if !ok || a.Kind != "command" {
			t.Fatal("not a command activity", provider, a)
		}
		if a.Shell != "zsh" || a.Command != "git status" {
			t.Fatalf("%s: shell %q command %q", provider, a.Shell, a.Command)
		}
	}
}
