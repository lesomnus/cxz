package server

import (
	"github.com/lesomnus/cxz/internal/core"
	"testing"
)

func TestBackgroundJournalProjection(t *testing.T) {
	raw := []byte(`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`)
	e := core.Event{Kind: "raw", Raw: raw, Seq: 7, RunID: "run"}
	v := pbEvent(e)
	if v.Kind != "background" || string(v.Payload) != string(raw) || v.Seq != 7 || v.RunId != "run" {
		t.Fatal(v)
	}
	if e.Kind != "raw" || e.Payload != nil {
		t.Fatal("journal mutated")
	}
	if pbEvent(core.Event{Kind: "raw", Raw: []byte(`{"type":"system","subtype":"init"}`)}).Kind != "raw" {
		t.Fatal("init misclassified")
	}
}
