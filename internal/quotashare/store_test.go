package quotashare

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentClaimsCacheAndTakeover(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	var wg sync.WaitGroup
	var winners atomic.Int32
	leases := make(chan string, 20)
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			out, err := Exchange(root, Request{Account: "shared"}, now)
			if err != nil {
				t.Error(err)
				return
			}
			if out.Poll {
				winners.Add(1)
				leases <- out.Snapshot.Lease
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("requests: %d", winners.Load())
	}
	lease := <-leases
	payload := json.RawMessage(`{"rate_limits":{"five_hour":{"utilization":20}}}`)
	published, err := Exchange(root, Request{Account: "shared", Lease: lease, Payload: payload}, now)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := Exchange(root, Request{Account: "shared"}, now.Add(time.Second))
	if err != nil || cached.Poll || compactJSON(cached.Snapshot.Payload) != compactJSON(payload) || cached.Snapshot.Observed != published.Snapshot.Observed {
		t.Fatal(cached, err)
	}
	other, _ := Exchange(root, Request{Account: "other"}, now)
	if !other.Poll || len(other.Snapshot.Payload) != 0 {
		t.Fatal("accounts mixed")
	}
	next, _ := Exchange(root, Request{Account: "shared"}, now.Add(time.Minute))
	if !next.Poll || next.Snapshot.Lease == lease {
		t.Fatal("no takeover")
	}
	stale, _ := Exchange(root, Request{Account: "shared", Lease: lease, Payload: json.RawMessage(`{}`)}, now.Add(time.Minute))
	if compactJSON(stale.Snapshot.Payload) != compactJSON(payload) || stale.Snapshot.Lease != "" {
		t.Fatal("stale publisher overwrote data or acquired capability")
	}
}
func TestQuotaTransportAuthorization(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "quota.sock")
	srv, err := Start(root, socket, func(_ context.Context, r Request, token string) bool { return token == "test" && r.Account == "shared" })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if _, err := Remote(context.Background(), socket, "wrong", Request{Account: "shared"}); err == nil {
		t.Fatal("unauthorized")
	}
	out, err := Remote(context.Background(), socket, "test", Request{Account: "shared"})
	if err != nil || !out.Poll {
		t.Fatal(out, err)
	}
}

func compactJSON(raw []byte) string {
	var out bytes.Buffer
	_ = json.Compact(&out, raw)
	return out.String()
}
