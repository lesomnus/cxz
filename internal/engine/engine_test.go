package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const owner = "123456789012345678901234"

func TestComposeMerge(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("Docker Compose unavailable")
	}
	e := Engine{Root: t.TempDir(), Owner: owner}
	s := Spec{Mode: "dind", Image: "docker:29-dind", Override: json.RawMessage(`{"services":{"dind":{"mem_limit":"2g","cpus":2,"volumes":["/srv/cache:/cache:ro"]}}}`)}
	b, revision, err := e.Render(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	json.Unmarshal(b, &cfg)
	d := cfg["services"].(map[string]any)["dind"].(map[string]any)
	if d["mem_limit"] != "2g" || len(d["volumes"].([]any)) != 2 || d["container_name"] != e.Name() {
		t.Fatal("override replaced managed base", string(b))
	}
	again, rev, err := e.Render(t.Context(), s)
	if err != nil || rev != revision || string(again) != string(b) {
		t.Fatal("render not stable", err)
	}
}
func TestOverrideValidation(t *testing.T) {
	for _, raw := range []string{`{"services":{"other":{}}}`, `{"services":{"dind":{"ports":["2375:2375"]}}}`, `{"services":{"dind":{"labels":{"cxz.owner":"foreign"}}}}`, `{"services":{"dind":{"volumes":["./relative:/cache"]}}}`, `{"services":{"dind":{"volumes":["${HOME}/cache:/cache"]}}}`, `{"services":{"dind":{"volumes":[{"type":"bind","source":"relative","target":"/cache"}]}}}`} {
		if (Spec{Mode: "dind", Override: json.RawMessage(raw)}).Validate() == nil {
			t.Fatal("accepted", raw)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docker.yaml"), []byte("services:\n  dind:\n    mem_limit: 2g\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := (Config{Mode: "dind", Compose: "docker.yaml"}).Snapshot(root)
	if err != nil || spec.Image != "docker:29-dind" {
		t.Fatal(spec, err)
	}
}
func TestOwnedEngineLifecycle(t *testing.T) {
	e := Engine{Root: t.TempDir(), Owner: owner}
	resources := map[string]bool{}
	var container map[string]any
	creates := 0
	ups := 0
	e.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		encode := func(v any) ([]byte, error) { return json.Marshal(v) }
		switch args[0] {
		case "ps":
			if container != nil {
				return []byte("engine-id"), nil
			}
			return nil, nil
		case "inspect":
			return encode([]any{container})
		case "volume", "network":
			name := args[len(args)-1]
			switch args[1] {
			case "ls":
				return nil, nil // creation must remain idempotent
			case "create":
				if !resources[name] {
					creates++
				}
				resources[name] = true
				return []byte(name), nil
			case "inspect":
				return encode([]any{map[string]any{"Labels": map[string]string{"cxz.owner": owner, "cxz.role": "docker-engine"}}})
			case "connect":
				return nil, nil
			}
		case "compose":
			var file string
			for i, v := range args {
				if v == "-f" {
					file = args[i+1]
				}
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			if strings.Contains(strings.Join(args, " "), " config ") {
				return b, nil
			}
			ups++
			var cfg map[string]any
			json.Unmarshal(b, &cfg)
			d := cfg["services"].(map[string]any)["dind"].(map[string]any)
			container = map[string]any{"Id": "engine-id", "Config": map[string]any{"Labels": d["labels"]}, "State": map[string]bool{"Running": true}}
			return nil, nil
		case "rm":
			container = nil
			return nil, nil
		}
		t.Fatalf("unexpected call %v", args)
		return nil, nil
	}
	s := Spec{Mode: "dind", Image: "docker:29-dind"}
	if err := e.Save(s); err != nil {
		t.Fatal(err)
	}
	saved, err := e.Load()
	if err != nil || saved.Image != s.Image {
		t.Fatal(saved, err)
	}
	if err = e.Ensure(t.Context(), s, false); err != nil {
		t.Fatal(err)
	}
	if err = e.Ensure(t.Context(), s, false); err != nil || creates != 2 {
		t.Fatal("resources not reused", err, creates)
	}
	s.Image = "docker:30-dind"
	if err = e.Ensure(t.Context(), s, false); err == nil || ups != 2 {
		t.Fatal("automatic replacement of live engine")
	}
	if err = e.Ensure(t.Context(), s, true); err != nil {
		t.Fatal(err)
	}
	if err = e.Down(t.Context()); err != nil || container != nil || len(resources) != 2 {
		t.Fatal("down deleted cache", err)
	}
	container = map[string]any{"Config": map[string]any{"Labels": map[string]string{"cxz.owner": "someone-else"}}}
	if err = e.Down(t.Context()); err == nil {
		t.Fatal("removed foreign container")
	}
}
