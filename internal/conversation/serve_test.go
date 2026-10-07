package conversation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The helper speaks the protocol its caller reads, and the index arrives before
// any session does -- that order is what lets a caller merge several projects
// without buffering all of them.
func TestScanHelperRoundTrip(t *testing.T) {
	c := newCorpus(t)
	c.session(t, "p", "first", "input", "the relay refused", 3*time.Hour)
	c.session(t, "p", "second", "input", "the relay answered", time.Hour)

	request, err := json.Marshal(ScanRequest{Project: "p", Query: ScanQuery{Query: "relay", Until: c.now}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = ServeScan(t.Context(), c.root, bytes.NewReader(request), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("%d lines: %q", len(lines), out.String())
	}
	var first ScanMessage
	if json.Unmarshal([]byte(lines[0]), &first) != nil || !first.Indexed || len(first.Index) != 2 {
		t.Fatalf("the index did not come first: %q", lines[0])
	}
	if first.Index[0].Title != "second" {
		t.Fatal("the index is not newest first:", first.Index[0].Title)
	}

	var visits []ScanVisit
	result, err := ReadScan(bytes.NewReader(out.Bytes()), func(v ScanVisit) error {
		visits = append(visits, v)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visits) != 2 || visits[0].Title != "second" || len(visits[0].Hits) != 1 {
		t.Fatalf("%+v", visits)
	}
	if result.Hits != 2 || result.Scanned != 2 {
		t.Fatalf("%+v", result)
	}
}

// A refused query is reported as a message and as an error, because the caller
// may have been given hits before it failed.
func TestScanHelperReportsRefusals(t *testing.T) {
	c := newCorpus(t)
	request, err := json.Marshal(ScanRequest{Project: "p", Query: ScanQuery{Query: "(", Match: "regex"}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = ServeScan(t.Context(), c.root, bytes.NewReader(request), &out); err == nil {
		t.Fatal("an invalid expression was accepted")
	}
	if _, err = ReadScan(bytes.NewReader(out.Bytes()), func(ScanVisit) error { return nil }); err == nil {
		t.Fatal("the reader did not see the refusal")
	}
	if err = ServeScan(t.Context(), c.root, strings.NewReader("not json"), &out); err == nil {
		t.Fatal("a malformed request was accepted")
	}
	if _, err = ReadScan(strings.NewReader(`{"what":1}`), func(ScanVisit) error { return nil }); err == nil {
		t.Fatal("an unrecognised line was skipped")
	}
}
