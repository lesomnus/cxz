package sessionstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
)

func TestLegacyJournalRemovalRequiresConversionAndCompleteCoverage(t *testing.T) {
	for _, scenario := range []string{"not-converted", "covered", "newer-journal", "torn-tail"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			conversation(t, dir)
			if scenario != "not-converted" {
				if _, err := Convert(t.Context(), dir, "s"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "newer-journal" {
				l, err := journal.Open(filepath.Join(dir, "events.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = l.Append(core.Event{SessionID: "s", Kind: "assistant", Text: "not converted"}); err != nil {
					t.Fatal(err)
				}
				l.Close()
			}
			if scenario == "torn-tail" {
				f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				f.WriteString("unfinished")
				f.Close()
			}
			removed, err := RemoveConvertedJournal(t.Context(), dir, "s")
			if err != nil || removed != (scenario == "covered") {
				t.Fatal(removed, err)
			}
			if scenario == "covered" {
				l, err := OpenLog(t.Context(), dir, "s")
				if err != nil {
					t.Fatal(err)
				}
				defer l.Close()
				if len(l.All()) == 0 {
					t.Fatal("conversation lost with legacy residue")
				}
			}
		})
	}
}
