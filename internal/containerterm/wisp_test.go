package containerterm

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/wisp"
)

func TestWispConnectionReuseAndCancelledRequest(t *testing.T) {
	in, write := io.Pipe()
	read, out := io.Pipe()
	defer in.Close()
	defer write.Close()
	defer read.Close()
	defer out.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _ = wisp.Serve(in, out) }()
	dec := json.NewDecoder(read)
	var hello wisp.Response
	if err := dec.Decode(&hello); err != nil {
		t.Fatal(err)
	}
	c := &wispClient{gate: make(chan struct{}, 1), enc: json.NewEncoder(write), dec: dec, done: done, stop: func() { write.Close(); read.Close() }}
	pool := &WispPool{clients: map[string]*wispClient{"p/c/u": c}}
	defer pool.Close()
	p := &api.Project{Id: "p", ContainerId: "c", RemoteUser: "u"}
	root := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(root, n), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	request, abort := context.WithCancel(ctx)
	_, _ = pool.Paths(ctx, request, p, root, func(PathListing) { abort() })
	for i := 0; i < 3; i++ {
		listing, err := pool.Paths(ctx, ctx, p, root, nil)
		if err != nil || len(listing.Entries) != 3 {
			t.Fatal(listing, err)
		}
		if pool.clients["p/c/u"] != c {
			t.Fatal("connection replaced")
		}
	}
}
