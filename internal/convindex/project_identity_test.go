package convindex

import (
	"github.com/lesomnus/cxz/internal/projectid"
	"testing"
	"time"
)

func TestUpgradeKeepsProjectSearchResults(t *testing.T) {
	c := newCorpus(t)
	legacy := "0123456789abcdef01234567"
	c.say(t, legacy, "session", "input", "retained project text", time.Second)
	if _, err := c.index.db.ExecContext(t.Context(), "UPDATE sessions SET project=?", legacy); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := c.index.migrateProjectIDs(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []string{legacy, projectid.Canonical(legacy)} {
		visits, _ := c.search(t, Query{Query: "retained", Projects: []string{ref}})
		if len(visits) != 1 {
			t.Fatal("project history disappeared after identity migration", ref, visits)
		}
	}
}
