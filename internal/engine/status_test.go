package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultEnabledAndExplicitOff(t *testing.T) {
	e := Engine{Root: t.TempDir(), Owner: owner}
	s, err := e.Load()
	if err != nil || s.Mode != "dind" || s.Image != "docker:29-dind" {
		t.Fatal(s, err)
	}
	s, err = (Config{}).Snapshot(e.Root)
	if err != nil || s.Mode != "dind" {
		t.Fatal(s, err)
	}
	if err = e.Save(Spec{Mode: "off"}); err != nil {
		t.Fatal(err)
	}
	s, err = e.Load()
	if err != nil || s.Mode != "off" {
		t.Fatal(s, err)
	}
}

func TestStatusAndPruneOwnedSocket(t *testing.T) {
	e := Engine{Root: t.TempDir(), Owner: owner}
	running, owned := true, true
	var executed []string
	e.Run = func(_ context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "ps":
			return []byte("id"), nil
		case "inspect":
			label := owner
			if !owned {
				label = "foreign"
			}
			return json.Marshal([]any{map[string]any{
				"Id":     "verified-id",
				"Config": map[string]any{"Image": "docker:29-dind", "Labels": map[string]string{"cxz.owner": label, "cxz.role": "docker-engine"}},
				"State":  map[string]any{"Running": running, "Health": map[string]string{"Status": "healthy"}},
			}})
		case "exec":
			executed = append(executed, strings.Join(args, " "))
			if args[5] == "system" {
				return []byte("{\"Type\":\"Images\",\"Size\":\"5GB\"}\n{\"Type\":\"Build Cache\",\"Size\":\"20MB\",\"Reclaimable\":\"10MB (50%)\"}\n"), nil
			}
			return []byte("Total reclaimed space: 10MB\n"), nil
		default:
			t.Fatalf("unexpected command %v", args)
		}
		return nil, nil
	}
	info, err := e.Info(t.Context())
	if err != nil || info.State != "running" || info.Health != "healthy" || info.BuildCache != "20MB" || info.Reclaimable != "10MB (50%)" {
		t.Fatal(info, err)
	}
	out, err := e.PruneBuildCache(t.Context())
	if err != nil || out != "Total reclaimed space: 10MB" {
		t.Fatal(out, err)
	}
	if len(executed) != 2 || executed[1] != "exec verified-id docker --host unix:///var/run/docker.sock builder prune --all --force" {
		t.Fatal(executed)
	}
	running = false
	if _, err = e.PruneBuildCache(t.Context()); err == nil {
		t.Fatal("pruned stopped engine")
	}
	running = true
	owned = false
	if _, err = e.PruneBuildCache(t.Context()); err == nil {
		t.Fatal("pruned foreign engine")
	}
	if len(executed) != 2 {
		t.Fatal(executed)
	}
}
