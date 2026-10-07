package convindex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/conversation"
)

// Query is one question about the conversation. The window is half-open,
// [Since, Until), so the windows a person walks back through -- a month at a
// time -- neither overlap nor skip.
type Query struct {
	Query        string
	Match        string // substring (default), regex or fuzzy
	IgnoreCase   bool
	IncludeTools bool
	Since        time.Time
	Until        time.Time
	Projects     []string // project ids; empty means every project
	Exclude      []string
	Sessions     []string
	Snippet      int
	Limit        int
	Resume       Resume
}

// Resume is the last row a page read, in the order every query reads: newest
// first, and within one instant by session and sequence. One tuple is enough
// because there is one store -- ordering is no longer assembled from several
// readers that each had to be told where they stopped.
type Resume struct {
	TimeMS  int64  `json:"time_ms,omitempty"`
	Session string `json:"session,omitempty"`
	Seq     uint64 `json:"seq,omitempty"`
}

func (r Resume) zero() bool { return r.Session == "" && r.TimeMS == 0 }

// Hit is one matching message.
type Hit struct {
	Seq     uint64
	Time    time.Time
	Kind    string
	Bytes   int
	Score   int
	Snippet string
	Cut     bool // The message was longer than the index keeps.
}

// Visit is one conversation's results, which is how a person reads them.
type Visit struct {
	Session   string
	Project   string
	Title     string
	Agent     string
	CreatedAt time.Time
	Activity  time.Time // This session's newest match in the window.
	Trimmed   bool
	Hits      []Hit
}

type Result struct {
	Sessions int
	Hits     int
	Examined int // Messages read to produce this page.
	Trimmed  int
	Since    time.Time
	Until    time.Time
	Next     *Resume
	HasMore  bool
}

// Cursor carries the window with the position, so that paging through one
// search asks the question the first page asked while events arrive above it.
type cursor struct {
	Version int    `json:"v"`
	SinceMS int64  `json:"since_ms,omitempty"`
	UntilMS int64  `json:"until_ms,omitempty"`
	Resume  Resume `json:"at"`
}

func EncodeCursor(since, until time.Time, r Resume) string {
	b, _ := json.Marshal(cursor{Version: 1, SinceMS: ms(since), UntilMS: ms(until), Resume: r})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor returns the window and position a cursor was taken at.
func DecodeCursor(s string) (since, until time.Time, r Resume, err error) {
	if len(s) > 1<<16 {
		return since, until, r, fmt.Errorf("cursor is too large")
	}
	b, e := base64.RawURLEncoding.DecodeString(s)
	var c cursor
	if e != nil || json.Unmarshal(b, &c) != nil || c.Version != 1 {
		return since, until, r, fmt.Errorf("invalid search cursor")
	}
	return at(c.SinceMS), at(c.UntilMS), c.Resume, nil
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func at(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func (q *Query) normalize(now time.Time) error {
	if q.Match == "" {
		q.Match = "substring"
	}
	if q.Match != "substring" && q.Match != "regex" && q.Match != "fuzzy" {
		return fmt.Errorf("match must be substring, regex or fuzzy")
	}
	if len(q.Query) > 4096 {
		return fmt.Errorf("query exceeds 4096 bytes")
	}
	if q.Match == "fuzzy" && strings.TrimSpace(q.Query) == "" {
		return fmt.Errorf("fuzzy matching needs something to match")
	}
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit {
		return fmt.Errorf("limit must be 1..%d", MaxLimit)
	}
	if q.Snippet == 0 {
		q.Snippet = conversation.DefaultSnippet
	}
	if q.Snippet < 0 {
		q.Snippet = 0
	}
	if q.Snippet > conversation.MaxSnippet {
		return fmt.Errorf("snippet must be at most %d bytes", conversation.MaxSnippet)
	}
	if q.Until.IsZero() {
		q.Until = now
	}
	if !q.Since.IsZero() && !q.Since.Before(q.Until) {
		return fmt.Errorf("since must precede until")
	}
	return nil
}

// Search answers in the order it was asked for, from one query.
//
// It reads rows newest first and groups them as they arrive, rather than
// ordering sessions before reading any of them. There is nothing to merge and
// nothing to wait for: a page is a few thousand rows of a table that holds only
// what was said, so it is assembled in the time a container would have taken to
// start.
func (i *Index) Search(ctx context.Context, q Query, now time.Time) ([]Visit, Result, error) {
	out := Result{}
	if err := q.normalize(now); err != nil {
		return nil, out, err
	}
	out.Since, out.Until = q.Since, q.Until
	m, err := conversation.NewMatcher(conversation.Match{Query: q.Query, Mode: q.Match, IgnoreCase: q.IgnoreCase})
	if err != nil {
		return nil, out, err
	}

	where := []string{"m.time_ms < ?"}
	args := []any{q.Until.UnixMilli()}
	if !q.Since.IsZero() {
		where = append(where, "m.time_ms >= ?")
		args = append(args, q.Since.UnixMilli())
	}
	kinds := []string{"input", "assistant"}
	if q.IncludeTools {
		kinds = Kinds
	}
	where = append(where, "m.kind IN ("+placeholders(len(kinds))+")")
	for _, k := range kinds {
		args = append(args, k)
	}
	if len(q.Projects) > 0 {
		where = append(where, "s.project IN ("+placeholders(len(q.Projects))+")")
		for _, v := range q.Projects {
			args = append(args, v)
		}
	}
	if len(q.Exclude) > 0 {
		where = append(where, "s.project NOT IN ("+placeholders(len(q.Exclude))+")")
		for _, v := range q.Exclude {
			args = append(args, v)
		}
	}
	if len(q.Sessions) > 0 {
		where = append(where, "m.session IN ("+placeholders(len(q.Sessions))+")")
		for _, v := range q.Sessions {
			args = append(args, v)
		}
	}
	// Substring is the question the store can answer itself. Regex and fuzzy
	// are not, and are applied to the rows it returns -- which is affordable
	// precisely because the rows are only the conversation.
	if q.Match == "substring" && q.Query != "" {
		if q.IgnoreCase {
			where = append(where, "instr(lower(m.text), lower(?)) > 0")
		} else {
			where = append(where, "instr(m.text, ?) > 0")
		}
		args = append(args, q.Query)
	}
	if !q.Resume.zero() {
		where = append(where, "(m.time_ms, m.session, m.seq) < (?, ?, ?)")
		args = append(args, q.Resume.TimeMS, q.Resume.Session, q.Resume.Seq)
	}

	// Substring is filtered by the store, so one row past the page is enough
	// to know whether there is more. Regex and fuzzy are filtered here, so the
	// bound is on rows examined instead: a page says what it read, and the
	// cursor carries on from there rather than from a guess.
	pushed := q.Match == "substring"
	limit := q.Limit
	rowBudget := limit + 1
	if !pushed {
		rowBudget = max(ScanBudget, limit+1)
	}
	query := `SELECT m.session, m.seq, m.time_ms, m.kind, m.text, m.cut,
		s.project, s.title, s.agent, s.created_ms, s.trimmed
		FROM messages m JOIN sessions s ON s.session = m.session
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY m.time_ms DESC, m.session DESC, m.seq DESC LIMIT ?`
	args = append(args, rowBudget)
	rows, err := i.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, out, err
	}
	defer rows.Close()

	var order []string
	visits := map[string]*Visit{}
	var emitted, examined Resume // The last row shown, and the last row read.
	seen := 0
	read := 0
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return nil, out, err
		}
		var session, kind, text, project, title, agent string
		var seq uint64
		var timeMS, createdMS int64
		var cut, trimmed int
		if err = rows.Scan(&session, &seq, &timeMS, &kind, &text, &cut, &project, &title, &agent, &createdMS, &trimmed); err != nil {
			return nil, out, err
		}
		read++
		examined = Resume{TimeMS: timeMS, Session: session, Seq: seq}
		score, lo, hi, ok := m.Find(text)
		if !ok {
			continue
		}
		if seen == limit {
			// One more match than there was room for: the page continues below
			// what it showed, not below what it read.
			out.HasMore = true
			out.Next = &emitted
			break
		}
		v := visits[session]
		if v == nil {
			v = &Visit{Session: session, Project: project, Title: title, Agent: agent,
				CreatedAt: at(createdMS), Activity: at(timeMS), Trimmed: trimmed != 0}
			visits[session] = v
			order = append(order, session)
			if v.Trimmed {
				out.Trimmed++
			}
		}
		hit := Hit{Seq: seq, Time: at(timeMS), Kind: kind, Bytes: len(text), Score: score, Cut: cut != 0}
		if q.Snippet > 0 {
			hit.Snippet = conversation.Snippet(text, lo, hi, q.Snippet)
		}
		v.Hits = append(v.Hits, hit)
		emitted = examined
		seen++
		out.Hits++
	}
	if err = rows.Err(); err != nil {
		return nil, out, err
	}
	if !out.HasMore && read >= rowBudget {
		// The budget ran out before the window did. Everything read is behind
		// the cursor, including rows the matcher refused.
		out.HasMore = true
		out.Next = &examined
	}
	out.Examined = read
	out.Sessions = len(order)
	result := make([]Visit, 0, len(order))
	for _, id := range order {
		result = append(result, *visits[id])
	}
	return result, out, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
