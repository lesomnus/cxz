package mcpconfig

import (
	"sync"
	"testing"
)

func TestDefaultsAndProjectOverrides(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"A", "B", "C"} {
		_, e := Apply(root, Request{Action: "put", ID: id, Server: &Server{Name: id, Kind: "stdio", Command: "test", Enabled: id != "C"}})
		if e != nil {
			t.Fatal(e)
		}
	}
	off, on := false, true
	for _, r := range []Request{{Action: "enable", ID: "B", Project: "P", Enabled: &off}, {Action: "enable", ID: "C", Project: "P", Enabled: &on}} {
		if _, e := Apply(root, r); e != nil {
			t.Fatal(e)
		}
	}
	check := func(project string, want ...string) {
		t.Helper()
		c, e := Load(root)
		if e != nil {
			t.Fatal(e)
		}
		v := c.Resolve(project)
		if len(v.Servers) != len(want) {
			t.Fatal(project, v)
		}
		for _, id := range want {
			if _, ok := v.Servers[id]; !ok {
				t.Fatal(project, id)
			}
		}
	}
	check("P", "A", "C")
	check("Q", "A", "B")
	_, _ = Apply(root, Request{Action: "enable", ID: "A", Enabled: &off})
	check("P", "C")
	check("Q", "B")
	_, _ = Apply(root, Request{Action: "enable", ID: "B", Project: "P"})
	check("P", "B", "C")
}
func TestConcurrentOverridesAndSecretRedaction(t *testing.T) {
	root := t.TempDir()
	_, e := Apply(root, Request{Action: "put", ID: "a", Server: &Server{Name: "a", Kind: "stdio", Command: "test", Env: map[string]string{"TOKEN": "secret"}}})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"p", "q", "r"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			on := true
			if _, e := Apply(root, Request{Action: "enable", ID: "a", Project: id, Enabled: &on}); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	c, e := Load(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"p", "q", "r"} {
		if len(c.Resolve(id).Servers) != 1 {
			t.Fatal(id)
		}
	}
	if c.View("").Entries[0].Server.Env != nil {
		t.Fatal("secret leaked")
	}
}
