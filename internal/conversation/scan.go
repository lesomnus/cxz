package conversation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/internal/core"
)

// Scanner searches every session one state root holds.
//
// It is deliberately a different type from Store, because it answers to a
// different person. An agent gets Store: one session, inside one project, whose
// manifest is checked twice, returning metadata and no text. The person who
// owns the installation gets this, over every session the root holds, with the
// matching text -- which is their own conversation, on their own screen.
//
// Nothing here consults the session registry, so it runs where the journals
// are: inside a project container, or in a short-lived helper that has the
// project's volume mounted read-only. Aliases and project names are added
// afterwards by whoever has them.
type Scanner struct {
	Root    string // a cxz state directory, holding sessions/<id>/events.jsonl
	Project string // when set, a session's manifest must agree
	Now     func() time.Time
}

const (
	// DefaultSnippet is enough to read the matching sentence, not the message.
	DefaultSnippet = 200
	MaxSnippet     = 2048
	// MaxScanHits bounds one session's page, and so the memory one session can
	// cost while its newest hits are collected from an oldest-first file.
	MaxScanHits = 500
)

// ScanQuery selects events by content and time across sessions. The window is
// half-open, [Since, Until), so consecutive windows neither overlap nor skip.
type ScanQuery struct {
	Query        string
	Match        string // substring (default), regex or fuzzy
	IgnoreCase   bool
	View         string // conversation (default) or raw
	IncludeTools bool
	Since        time.Time // zero: no lower bound
	Until        time.Time // zero: up to the scan's start
	Sessions     []string  // runtime ids; empty means every session
	Snippet      int       // matching text budget per hit; negative means none
	Limit        int       // hits per session, 1..MaxScanHits
	Resume       Resume
}

// Resume continues a scan where a page stopped. The session order is keyed on
// the time of each session's newest event *within the window*, which events
// arriving after Until cannot change -- so the next page of a fixed window sees
// the same order this one did.
type Resume struct {
	Activity int64  `json:"activity_ms,omitempty"`
	Session  string `json:"session,omitempty"`
	Seq      uint64 `json:"seq,omitempty"`
}

func (r Resume) zero() bool { return r.Session == "" }

// ScanCursor continues one window over however many roots a search covered. It
// is keyed by project because each project's journals are read separately: what
// one has finished says nothing about the others, and an installation with a
// single root is just the one key.
//
// The window travels in the cursor so that paging through it cannot be shifted
// by events arriving while a person reads: those are above Until, and the next
// page asks the same question the first one did.
type ScanCursor struct {
	Version int               `json:"v"`
	SinceMS int64             `json:"since_ms,omitempty"`
	UntilMS int64             `json:"until_ms,omitempty"`
	At      map[string]Resume `json:"at,omitempty"`
	Done    map[string]bool   `json:"done,omitempty"`
}

func (c ScanCursor) Encode() string {
	c.Version = 1
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeScanCursor(s string) (ScanCursor, error) {
	var c ScanCursor
	if len(s) > 1<<16 {
		return c, fmt.Errorf("cursor is too large")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, &c) != nil || c.Version != 1 {
		return c, fmt.Errorf("invalid search cursor")
	}
	return c, nil
}

// Window reports the half-open range a cursor was taken in.
func (c ScanCursor) Window() (since, until time.Time) { return MSTime(c.SinceMS), MSTime(c.UntilMS) }

func MSTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func TimeMS(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// ScanSession is what the journals know about a session on their own. The
// alias, the project's name and the session's current state live elsewhere.
type ScanSession struct {
	ID        string    `json:"session"`
	Title     string    `json:"title,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Project   string    `json:"project,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	Activity  time.Time `json:"activity"`
	// Truncated says older events are gone, so an absence of hits in this
	// session is not evidence that nothing was said.
	Truncated bool `json:"truncated,omitempty"`
}

// ScanHit is one matching event. Which session it belongs to is said once, by
// the visit that carries it, rather than again on every hit.
type ScanHit struct {
	Seq     uint64    `json:"seq"`
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Bytes   int       `json:"bytes"`
	Score   int       `json:"score,omitempty"`
	Snippet string    `json:"snippet,omitempty"`
}

// ScanVisit is one session's page: the session, and what was found in it.
//
// A session at a time is the unit this streams in, because a journal is read
// forwards and which matches are the newest is only known at its end. It is
// also how a result list wants them -- grouped, under the conversation they
// were said in.
type ScanVisit struct {
	ScanSession
	Hits []ScanHit `json:"hits,omitempty"`
}

// ScanResult reports what the scan covered, which is as much of the answer as
// the hits are: a window with nothing in it and a window nobody finished
// reading look the same in a list of results.
type ScanResult struct {
	Sessions  int     `json:"sessions"`
	Scanned   int     `json:"scanned"`
	Hits      int     `json:"hits"`
	Truncated int     `json:"truncated,omitempty"`
	Next      *Resume `json:"next,omitempty"`
}

func (s *Scanner) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (q *ScanQuery) normalize(now time.Time) error {
	if q.View == "" {
		q.View = "conversation"
	}
	if q.View != "conversation" && q.View != "raw" {
		return fmt.Errorf("view must be conversation or raw")
	}
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
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > MaxScanHits {
		return fmt.Errorf("limit must be 1..%d", MaxScanHits)
	}
	if q.Snippet == 0 {
		q.Snippet = DefaultSnippet
	}
	if q.Snippet < 0 {
		q.Snippet = 0
	}
	if q.Snippet > MaxSnippet {
		return fmt.Errorf("snippet must be at most %d bytes", MaxSnippet)
	}
	if q.Until.IsZero() {
		q.Until = now
	}
	if !q.Since.IsZero() && !q.Since.Before(q.Until) {
		return fmt.Errorf("since must precede until")
	}
	if len(q.Sessions) > MaxScanHits {
		return fmt.Errorf("too many sessions named")
	}
	return nil
}

// Index lists the sessions a query will read, in the order it will read them
// and including a resumed page's skips, without reading any of them. It is a
// tail read per session, so a caller that has to order several roots against
// each other can learn the order first and cheaply.
func (s *Scanner) Index(ctx context.Context, q ScanQuery) ([]ScanSession, error) {
	if err := q.normalize(s.now()); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return s.index(ctx, root, q)
}

// Scan emits one visit per session, newest session first, each holding its
// newest matches first, and returns where it stopped.
//
// Order is the scan order rather than a sort: a consumer may draw every hit as
// it arrives and never has to move one it has already drawn.
func (s *Scanner) Scan(ctx context.Context, q ScanQuery, emit func(ScanVisit) error) (ScanResult, error) {
	sessions, err := s.Index(ctx, q)
	if err != nil {
		return ScanResult{}, err
	}
	return s.ScanIndexed(ctx, q, sessions, emit)
}

// ScanIndexed reads the sessions an earlier Index call listed.
func (s *Scanner) ScanIndexed(ctx context.Context, q ScanQuery, sessions []ScanSession, emit func(ScanVisit) error) (ScanResult, error) {
	var out ScanResult
	if err := q.normalize(s.now()); err != nil {
		return out, err
	}
	m, err := newMatcher(q)
	if err != nil {
		return out, err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return out, err
	}
	defer root.Close()
	out.Sessions = len(sessions)
	budget := q.Limit
	for _, v := range sessions {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		below := uint64(0)
		if v.ID == q.Resume.Session {
			below = q.Resume.Seq
		}
		hits, truncated, err := s.session(ctx, root, v, q, m, below, budget)
		if err != nil {
			return out, err
		}
		out.Scanned++
		if truncated {
			out.Truncated++
			v.Truncated = true
		}
		if len(hits) > budget {
			hits = hits[:budget]
		}
		if err = emit(ScanVisit{ScanSession: v, Hits: hits}); err != nil {
			return out, err
		}
		out.Hits += len(hits)
		budget -= len(hits)
		if budget == 0 {
			// A page that ends exactly on a session's last hit still resumes
			// there: the next page reads below that sequence, finds nothing,
			// and goes on -- one session re-read, nothing lost.
			out.Next = &Resume{Activity: v.Activity.UnixMilli(), Session: v.ID, Seq: hits[len(hits)-1].Seq}
			return out, nil
		}
	}
	return out, nil
}

// index lists the sessions worth reading, in the order they will be read. A
// session whose newest event predates the window is dropped here, which is what
// makes an older window cheap rather than a second full pass.
func (s *Scanner) index(ctx context.Context, root *os.Root, q ScanQuery) ([]ScanSession, error) {
	entries, err := fs.ReadDir(root.FS(), "sessions")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	only := map[string]bool{}
	for _, id := range q.Sessions {
		only[id] = true
	}
	var out []ScanSession
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		id := entry.Name()
		if !entry.IsDir() || !runtimeID.MatchString(id) || (len(only) > 0 && !only[id]) {
			continue
		}
		b, err := root.ReadFile(filepath.Join("sessions", id, "session.json"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var manifest core.Session
		if json.Unmarshal(b, &manifest) != nil || manifest.ID != id {
			continue
		}
		if s.Project != "" && manifest.ProjectID != s.Project {
			continue
		}
		activity, err := s.activity(root, id, q.Until)
		if err != nil {
			return nil, err
		}
		if activity.IsZero() || (!q.Since.IsZero() && activity.Before(q.Since)) {
			continue
		}
		// Everything above the resume point was read by an earlier page. It is
		// dropped here so that the index is exactly the sessions that will be
		// visited, which is what a caller merging several roots relies on.
		if ms := activity.UnixMilli(); !q.Resume.zero() && (ms > q.Resume.Activity || (ms == q.Resume.Activity && id > q.Resume.Session)) {
			continue
		}
		v := ScanSession{ID: id, Title: manifest.Title, Agent: manifest.Kind, Project: manifest.ProjectID, Activity: activity}
		if manifest.CreatedAt > 0 {
			v.CreatedAt = time.UnixMilli(manifest.CreatedAt).UTC()
		}
		if v.Agent == "" {
			v.Agent = manifest.Agent
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Activity.Equal(out[j].Activity) {
			return out[i].Activity.After(out[j].Activity)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// activity is the time of the newest event before until, read from the end of
// the journal rather than by scanning it. A session with nothing before until
// reports the zero time, and is then not read at all -- which is what makes an
// older window cost the sessions that have something in it rather than all of
// them.
//
// It reads the end because a journal is written as things happen, so the end is
// the newest. That assumption is only load-bearing at a window's edge: a clock
// stepping backwards mid-session could put an event a little out of order and
// move a session between windows, never into neither.
func (s *Scanner) activity(root *os.Root, id string, until time.Time) (time.Time, error) {
	f, err := root.Open(filepath.Join("sessions", id, "events.jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return time.Time{}, err
	}
	const chunk = 64 << 10
	end := info.Size()
	var tail []byte
	for end > 0 {
		size := int64(chunk)
		if size > end {
			size = end
		}
		buf := make([]byte, size)
		if _, err = f.ReadAt(buf, end-size); err != nil && err != io.EOF {
			return time.Time{}, err
		}
		tail = append(buf, tail...)
		end -= size
		// Walk whole lines from the end; a record may be one event or a batch.
		for {
			cut := bytes.LastIndexByte(tail[:max(0, len(tail)-1)], '\n')
			line := tail
			if cut >= 0 {
				line = tail[cut+1:]
			} else if end > 0 {
				break // The first line in hand may be a fragment.
			}
			if t, ok := newestBefore(line, until); ok {
				return t, nil
			}
			if cut < 0 {
				break
			}
			tail = tail[:cut]
		}
		if len(tail) > 8<<20 {
			return time.Time{}, fmt.Errorf("journal record exceeds 8 MiB")
		}
	}
	return time.Time{}, nil
}

func newestBefore(line []byte, until time.Time) (time.Time, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return time.Time{}, false
	}
	var batch []core.Event
	if line[0] == '[' {
		if json.Unmarshal(line, &batch) != nil {
			return time.Time{}, false
		}
	} else {
		var e core.Event
		if json.Unmarshal(line, &e) != nil {
			return time.Time{}, false
		}
		batch = []core.Event{e}
	}
	for i := len(batch) - 1; i >= 0; i-- {
		t := time.UnixMilli(batch[i].TimeMS).UTC()
		if batch[i].TimeMS > 0 && t.Before(until) {
			return t, true
		}
	}
	return time.Time{}, false
}

// session reads one journal forward -- the only direction a file of appended
// records can be read -- and keeps the newest matches, so the caller receives
// the page a person wants from a file ordered the way a log has to be.
func (s *Scanner) session(ctx context.Context, root *os.Root, v ScanSession, q ScanQuery, m *matcher, below uint64, budget int) ([]ScanHit, bool, error) {
	f, err := root.Open(filepath.Join("sessions", v.ID, "events.jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	keep := min(budget, q.Limit)
	ring := make([]ScanHit, 0, keep+1)
	truncated := false
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 32<<20)
	for scan.Scan() {
		if err = ctx.Err(); err != nil {
			return nil, false, err
		}
		line := bytes.TrimSpace(scan.Bytes())
		if len(line) == 0 {
			continue
		}
		var batch []core.Event
		if line[0] == '[' {
			err = json.Unmarshal(line, &batch)
		} else {
			var e core.Event
			err = json.Unmarshal(line, &e)
			batch = []core.Event{e}
		}
		if err != nil {
			return nil, false, fmt.Errorf("session %s: invalid committed journal record", v.ID)
		}
		for _, e := range batch {
			if e.Kind == core.HistoryCheckpointKind || core.HistoryFloor(e.Kind, e.Payload) > 0 {
				truncated = true
				continue
			}
			if below > 0 && e.Seq >= below {
				continue
			}
			t := time.UnixMilli(e.TimeMS).UTC()
			if !t.Before(q.Until) || (!q.Since.IsZero() && t.Before(q.Since)) {
				continue
			}
			out, ok := rendered(e, Query{View: q.View, IncludeTools: q.IncludeTools})
			if !ok {
				continue
			}
			score, lo, hi, ok := m.match(target(out, q.View))
			if !ok {
				continue
			}
			full, _ := json.Marshal(out)
			hit := ScanHit{Seq: out.Seq, Time: out.Time, Kind: out.Kind, Bytes: len(full), Score: score}
			if q.Snippet > 0 {
				hit.Snippet = snippet(target(out, q.View), lo, hi, q.Snippet)
			}
			ring = append(ring, hit)
			if len(ring) > keep {
				ring = ring[1:]
			}
		}
	}
	if err = scan.Err(); err != nil {
		return nil, false, err
	}
	for i, j := 0, len(ring)-1; i < j; i, j = i+1, j-1 {
		ring[i], ring[j] = ring[j], ring[i]
	}
	return ring, truncated, nil
}

// target is the text a query is matched against, which includes the structured
// payload: a tool call's arguments are where its subject usually is.
func target(e Event, view string) string {
	if view == "raw" {
		if len(e.Raw) > 0 {
			return string(e.Raw)
		}
		return string(e.RawBase64)
	}
	if len(e.Payload) > 0 {
		return e.Text + "\n" + string(e.Payload)
	}
	return e.Text
}

type matcher struct {
	re         *regexp.Regexp
	query      string
	fuzzy      bool
	ignoreCase bool
}

func newMatcher(q ScanQuery) (*matcher, error) {
	m := &matcher{query: q.Query, fuzzy: q.Match == "fuzzy", ignoreCase: q.IgnoreCase}
	if q.Match == "regex" {
		pattern := q.Query
		if q.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid RE2 expression: %w", err)
		}
		m.re = re
	}
	if m.ignoreCase {
		m.query = strings.ToLower(m.query)
	}
	return m, nil
}

// match reports the score and the byte span that matched, which is what a
// snippet is cut around. An empty substring query matches everything, as it
// does in the per-session search, so a time range alone is a valid question.
func (m *matcher) match(text string) (score, lo, hi int, ok bool) {
	switch {
	case m.re != nil:
		span := m.re.FindStringIndex(text)
		if span == nil {
			return 0, 0, 0, false
		}
		return 0, span[0], span[1], true
	case m.fuzzy:
		return fuzzyMatch(text, m.query, m.ignoreCase)
	default:
		if m.query == "" {
			return 0, 0, 0, true
		}
		haystack := text
		if m.ignoreCase {
			haystack = strings.ToLower(haystack)
		}
		i := strings.Index(haystack, m.query)
		if i < 0 {
			return 0, 0, 0, false
		}
		return 0, i, i + len(m.query), true
	}
}

// snippet is the matching text, with enough either side to read it as a
// sentence. Whitespace is collapsed because a transcript is full of it and a
// result list has one line.
func snippet(text string, lo, hi, budget int) string {
	if budget <= 0 || text == "" {
		return ""
	}
	lo = min(max(lo, 0), len(text))
	hi = min(max(hi, lo), len(text))
	span := hi - lo
	if span > budget {
		hi = lo + budget
		span = budget
	}
	slack := budget - span
	start := lo - slack/2
	if start < 0 {
		start = 0
	}
	end := min(len(text), start+budget)
	start = max(0, min(start, end-budget))
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	var b strings.Builder
	if start > 0 {
		b.WriteRune('…')
	}
	space := false
	for _, r := range text[start:end] {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	if end < len(text) {
		b.WriteRune('…')
	}
	return b.String()
}
