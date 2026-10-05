// Package conversation exposes bounded, project-scoped reads of cxz journals.
package conversation

import (
	"context"
	"encoding/json"
	"time"
)

const DefaultBytes = 32 << 10
const MaxInlineBytes = 128 << 10
const MaxFileBytes = 16 << 20

type Session struct {
	ID        string `json:"session_id"`
	Alias     string `json:"session_alias"`
	Agent     string `json:"agent"`
	State     string `json:"state"`
	RuntimeID string `json:"-"`
	ProjectID string `json:"-"`
}
type Query struct {
	Session      string   `json:"session,omitempty" jsonschema:"Session alias or UUID, without @. Use the returned session_id UUID for subsequent reads."`
	Query        string   `json:"query,omitempty" jsonschema:"Text or RE2 expression. Empty matches all selected events."`
	Match        string   `json:"match,omitempty" jsonschema:"substring (default) or regex (Go RE2)"`
	IgnoreCase   bool     `json:"ignore_case,omitempty"`
	View         string   `json:"view,omitempty" jsonschema:"conversation (default) or raw vendor events; raw never reads provider files or secret references"`
	IncludeTools bool     `json:"include_tools,omitempty" jsonschema:"Include tool calls/results in conversation view"`
	Since        string   `json:"since,omitempty" jsonschema:"Inclusive RFC3339 timestamp"`
	Until        string   `json:"until,omitempty" jsonschema:"Exclusive RFC3339 timestamp"`
	Around       string   `json:"around,omitempty" jsonschema:"RFC3339 timestamp or relative server time such as 5m ago; selects +/- window (default 2m)"`
	Window       string   `json:"window,omitempty" jsonschema:"Duration around the selected time, at most 24h"`
	Seqs         []uint64 `json:"seqs,omitempty" jsonschema:"Read exact event sequences; missing sequences are reported"`
	Before       int      `json:"before,omitempty" jsonschema:"Include up to 100 preceding selected messages around seqs"`
	After        int      `json:"after,omitempty" jsonschema:"Include up to 100 following selected messages around seqs"`
	LineStart    int      `json:"line_start,omitempty" jsonschema:"1-based message text line, only with one seq in conversation view"`
	LineEnd      int      `json:"line_end,omitempty" jsonschema:"Inclusive last message text line"`
	SnapshotSeq  *uint64  `json:"snapshot_seq,omitempty" jsonschema:"Upper event sequence from a prior search/read"`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"Opaque continuation from this tool; reuses its UUID, filters and snapshot"`
	Limit        int      `json:"limit,omitempty" jsonschema:"Event page size; default 50, maximum 500"`
	Output       string   `json:"output,omitempty" jsonschema:"inline (default) or file (read only); file is accessible in the calling session's container"`
	MaxBytes     int      `json:"max_bytes,omitempty" jsonschema:"Body budget: inline default 32768 max 131072; file default/max 16777216"`
}
type Event struct {
	Seq       uint64          `json:"seq"`
	Time      time.Time       `json:"time"`
	Kind      string          `json:"kind"`
	Text      string          `json:"text,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	RawBase64 []byte          `json:"raw_base64,omitempty"`
	LineStart int             `json:"line_start,omitempty"`
	LineEnd   int             `json:"line_end,omitempty"`
}
type Hit struct {
	Seq   uint64    `json:"seq"`
	Time  time.Time `json:"time"`
	Bytes int       `json:"bytes"`
}
type Reply struct {
	Session
	ObservedAt       time.Time  `json:"observed_at"`
	SnapshotSeq      uint64     `json:"snapshot_seq"`
	FirstSeq         uint64     `json:"first_seq"`
	FirstTime        *time.Time `json:"first_time,omitempty"`
	LastActivity     *time.Time `json:"last_activity,omitempty"`
	TrimmedThrough   uint64     `json:"trimmed_through,omitempty"`
	HistoryTruncated bool       `json:"history_truncated"`
	View             string     `json:"view,omitempty"`
	Since            string     `json:"since,omitempty"`
	Until            string     `json:"until,omitempty"`
	Hits             []Hit      `json:"events,omitempty"`
	Bodies           []Event    `json:"messages,omitempty"`
	ReturnedBytes    int        `json:"returned_bytes"`
	MissingSeqs      []uint64   `json:"missing_seqs,omitempty"`
	UnavailableSeqs  []uint64   `json:"unavailable_seqs,omitempty"`
	OversizedSeqs    []uint64   `json:"oversized_seqs,omitempty"`
	NextCursor       string     `json:"next_cursor,omitempty"`
	HasMore          bool       `json:"has_more"`
	Path             string     `json:"path,omitempty"`
	MetadataPath     string     `json:"metadata_path,omitempty"`
	EventCount       int        `json:"event_count,omitempty"`
	FileBytes        int        `json:"file_bytes,omitempty"`
	Message          string     `json:"message,omitempty"`
}
type Registry func(context.Context) ([]Session, error)
type Store struct {
	Root     string
	Caller   string
	Project  string
	Registry Registry
	Now      func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
