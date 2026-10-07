package convindex

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/payday/config"
)

// TestRealCorpus measures the index against a real installation's event store,
// which is the only way to know whether extracting the conversation is worth
// anything. It is skipped unless CXZ_REAL_STATE names one.
func TestRealCorpus(t *testing.T) {
	root := os.Getenv("CXZ_REAL_STATE")
	if root == "" {
		t.Skip("set CXZ_REAL_STATE to a cxz state directory to measure against it")
	}
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + root + "/cxz.db?mode=ro", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	index, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()

	rows, err := db.QueryContext(ctx, "SELECT session_id, seq, data FROM events ORDER BY session_id, seq")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	start := time.Now()
	events := 0
	bytes := 0
	batch := map[string][]core.Event{}
	spans := map[string][2]uint64{}
	flush := func() {
		for id, list := range batch {
			span := spans[id]
			if err := index.Ingest(ctx, Session{ID: id}, list, span[0], span[1]); err != nil {
				t.Fatal(err)
			}
		}
		batch = map[string][]core.Event{}
		spans = map[string][2]uint64{}
	}
	pending := 0
	for rows.Next() {
		var id string
		var seq uint64
		var data []byte
		if err = rows.Scan(&id, &seq, &data); err != nil {
			t.Fatal(err)
		}
		events++
		bytes += len(data)
		var e core.Event
		if json.Unmarshal(data, &e) != nil {
			continue
		}
		e.SessionID, e.Seq = id, seq
		batch[id] = append(batch[id], e)
		span := spans[id]
		if span[0] == 0 || seq < span[0] {
			span[0] = seq
		}
		if seq > span[1] {
			span[1] = seq
		}
		spans[id] = span
		if pending++; pending >= 2000 {
			flush()
			pending = 0
		}
	}
	flush()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	ingest := time.Since(start)

	var messages, textBytes int
	if err = index.db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(LENGTH(text)),0) FROM messages").Scan(&messages, &textBytes); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(root + "/cxz.db")
	MiB := 1048576.0
	t.Logf("store: %d events, %.1f MiB of event JSON (file %.1f MiB)", events, float64(bytes)/MiB, float64(info.Size())/MiB)
	t.Logf("index: %d messages, %.2f MiB of text, built in %s", messages, float64(textBytes)/MiB, ingest.Round(time.Millisecond))

	now := time.Now().UTC()
	for _, c := range []struct {
		label string
		q     Query
	}{
		{"substring, 30 days", Query{Query: "relay", Since: now.Add(-30 * 24 * time.Hour)}},
		{"substring, all time", Query{Query: "relay"}},
		{"substring, no match", Query{Query: "zzzz-no-such-text-zzzz"}},
		{"regex, all time", Query{Query: "relay.*certificate", Match: "regex", IgnoreCase: true}},
		{"fuzzy, all time", Query{Query: "cxzweb", Match: "fuzzy", IgnoreCase: true}},
		{"with tools", Query{Query: "relay", IncludeTools: true}},
		{"empty query, 1 day", Query{Since: now.Add(-24 * time.Hour)}},
	} {
		best := time.Hour
		var out Result
		for i := 0; i < 3; i++ {
			at := time.Now()
			if _, out, err = index.Search(ctx, c.q, now); err != nil {
				t.Fatal(c.label, err)
			}
			if d := time.Since(at); d < best {
				best = d
			}
		}
		t.Logf("%-22s %8s  hits=%d sessions=%d examined=%d", c.label, best.Round(time.Microsecond), out.Hits, out.Sessions, out.Examined)
	}
}
