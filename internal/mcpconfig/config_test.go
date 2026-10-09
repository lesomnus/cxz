package mcpconfig

import (
	"sync"
	"testing"
)

func TestDefaultsAndProjectOverrides(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"A", "B", "C"} {
		_, e := Put(root, id, Server{Name: id, Kind: "stdio", Command: "test", Enabled: id != "C"})
		if e != nil {
			t.Fatal(e)
		}
	}
	if _, e := SetProject(root, "P", "B", false); e != nil {
		t.Fatal(e)
	}
	if _, e := SetProject(root, "P", "C", true); e != nil {
		t.Fatal(e)
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
	if _, e := SetDefault(root, "A", false); e != nil {
		t.Fatal(e)
	}
	check("P", "C")
	check("Q", "B")
	// Restoring the default is its own call now; it used to be the same verb
	// with the on/off left out.
	if _, e := ClearProject(root, "P", "B"); e != nil {
		t.Fatal(e)
	}
	check("P", "B", "C")
}
func TestConcurrentOverridesAndSecretRedaction(t *testing.T) {
	root := t.TempDir()
	_, e := Put(root, "a", Server{Name: "a", Kind: "stdio", Command: "test", Env: map[string]string{"TOKEN": "secret"}})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"p", "q", "r"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := SetProject(root, id, "a", true); e != nil {
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
