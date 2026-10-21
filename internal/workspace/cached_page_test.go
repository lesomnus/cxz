package workspace

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
)

// cachedPage reads the manager's own events table. The cache's trim rules and
// its refusal of frames from removed sessions are still its own, but nothing
// reads it as history any more -- raw history comes from the session's store --
// so the tests that are about the table state the query themselves.
// See cxz#268.
func cachedPage(t *testing.T, db *sql.DB, session string, after uint64) []*api.Event {
	t.Helper()
	rows, err := db.Query("SELECT data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT 200", session, after)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []*api.Event
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		var v api.Event
		if err = json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, &v)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
