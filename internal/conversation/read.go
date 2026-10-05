package conversation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

var runtimeID = regexp.MustCompile(`^[a-f0-9]{24}$`)

type continuation struct {
	Version   int    `json:"v"`
	Operation string `json:"op"`
	Query     Query  `json:"q"`
	Last      uint64 `json:"last"`
}

func (s *Store) resolve(ctx context.Context, handle string) (Session, error) {
	if handle == "" || strings.HasPrefix(handle, "@") {
		return Session{}, fmt.Errorf("session alias or UUID required, without @")
	}
	items, err := s.Registry(ctx)
	if err != nil {
		return Session{}, err
	}
	var matches []Session
	for _, v := range items {
		if v.ProjectID == s.Project && (v.ID == handle || v.Alias == handle) {
			matches = append(matches, v)
		}
	}
	if len(matches) != 1 {
		return Session{}, fmt.Errorf("session not found or ambiguous in this project")
	}
	if !runtimeID.MatchString(matches[0].RuntimeID) {
		return Session{}, fmt.Errorf("invalid runtime identity")
	}
	return matches[0], nil
}

// Read committed records only. os.Root prevents a session symlink escaping the
// project; explicit limits also bound malformed/oversized journals before pruning.
func (s *Store) journal(ctx context.Context, v Session) ([]core.Event, error) {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir := filepath.Join("sessions", v.RuntimeID)
	b, err := root.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return nil, err
	}
	var manifest core.Session
	if json.Unmarshal(b, &manifest) != nil || manifest.ID != v.RuntimeID || manifest.ProjectID != s.Project {
		return nil, fmt.Errorf("session manifest outside project scope")
	}
	f, err := root.Open(filepath.Join(dir, "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 32<<20)
	scan.Split(func(data []byte, eof bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[:i], nil
		}
		if eof {
			return len(data), nil, nil
		}
		return 0, nil, nil
	})
	var out []core.Event
	total := 0
	var last uint64
	for scan.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scan.Bytes()
		total += len(line) + 1
		if total > 256<<20 {
			return nil, fmt.Errorf("journal exceeds 256 MiB query scan limit")
		}
		var batch []core.Event
		if len(line) > 0 && line[0] == '[' {
			err = json.Unmarshal(line, &batch)
		} else {
			var e core.Event
			err = json.Unmarshal(line, &e)
			batch = []core.Event{e}
		}
		if err != nil {
			return nil, fmt.Errorf("invalid committed journal record")
		}
		for _, e := range batch {
			if e.Seq <= last || (e.SessionID != "" && e.SessionID != v.RuntimeID) {
				return nil, fmt.Errorf("invalid journal identity or sequence")
			}
			last = e.Seq
			out = append(out, e)
		}
	}
	return out, scan.Err()
}
func details(v Session, events []core.Event, now time.Time) Reply {
	r := Reply{Session: v, ObservedAt: now}
	for _, e := range events {
		r.SnapshotSeq = e.Seq
		t := time.UnixMilli(e.TimeMS).UTC()
		r.LastActivity = &t
		if e.Kind == core.HistoryCheckpointKind {
			r.TrimmedThrough = max(r.TrimmedThrough, e.Seq)
			continue
		}
		r.TrimmedThrough = max(r.TrimmedThrough, core.HistoryFloor(e.Kind, e.Payload))
		if r.FirstSeq == 0 {
			r.FirstSeq = e.Seq
			r.FirstTime = &t
		}
	}
	r.HistoryTruncated = r.TrimmedThrough > 0
	return r
}
func (s *Store) Lookup(ctx context.Context, handle string) (Reply, error) {
	v, err := s.resolve(ctx, handle)
	if err != nil {
		return Reply{}, err
	}
	events, err := s.journal(ctx, v)
	if err != nil {
		return Reply{}, err
	}
	return details(v, events, s.now()), nil
}
func normalize(q *Query, now time.Time) error {
	if q.View == "" {
		q.View = "conversation"
	}
	if q.View != "conversation" && q.View != "raw" {
		return fmt.Errorf("view must be conversation or raw")
	}
	if q.Match == "" {
		q.Match = "substring"
	}
	if q.Match != "substring" && q.Match != "regex" {
		return fmt.Errorf("match must be substring or regex")
	}
	if len(q.Query) > 4096 || len(q.Seqs) > 500 {
		return fmt.Errorf("query or seqs exceeds limit")
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > 500 {
		return fmt.Errorf("limit must be 1..500")
	}
	if q.Before < 0 || q.Before > 100 || q.After < 0 || q.After > 100 {
		return fmt.Errorf("before/after must be 0..100")
	}
	if (q.Before > 0 || q.After > 0) && len(q.Seqs) == 0 {
		return fmt.Errorf("before/after requires seqs")
	}
	if q.LineStart < 0 || q.LineEnd < 0 || (q.LineEnd > 0 && q.LineEnd < q.LineStart) {
		return fmt.Errorf("invalid line range")
	}
	if (q.LineStart > 0 || q.LineEnd > 0) && (len(q.Seqs) != 1 || q.View != "conversation" || q.Before > 0 || q.After > 0) {
		return fmt.Errorf("line range requires exactly one seq, conversation view and no context expansion")
	}
	if q.Around != "" {
		if q.Since != "" || q.Until != "" {
			return fmt.Errorf("around cannot be combined with since/until")
		}
		t, err := time.Parse(time.RFC3339Nano, q.Around)
		if strings.HasSuffix(q.Around, " ago") {
			var d time.Duration
			d, err = time.ParseDuration(strings.TrimSuffix(q.Around, " ago"))
			if err == nil && d < 0 {
				err = fmt.Errorf("negative relative duration")
			}
			t = now.Add(-d)
		}
		if err != nil {
			return fmt.Errorf("around must be RFC3339 or a duration such as 5m ago")
		}
		window := 2 * time.Minute
		if q.Window != "" {
			window, err = time.ParseDuration(q.Window)
			if err != nil {
				return err
			}
		}
		if window <= 0 || window > 24*time.Hour {
			return fmt.Errorf("window must be positive and at most 24h")
		}
		q.Since = t.Add(-window).Format(time.RFC3339Nano)
		q.Until = t.Add(window).Format(time.RFC3339Nano)
		q.Around = ""
		q.Window = ""
	} else if q.Window != "" {
		return fmt.Errorf("window requires around")
	}
	var lo, hi time.Time
	var err error
	if q.Since != "" {
		lo, err = time.Parse(time.RFC3339Nano, q.Since)
		if err != nil {
			return fmt.Errorf("invalid since timestamp")
		}
	}
	if q.Until != "" {
		hi, err = time.Parse(time.RFC3339Nano, q.Until)
		if err != nil {
			return fmt.Errorf("invalid until timestamp")
		}
	}
	if !lo.IsZero() && !hi.IsZero() && !lo.Before(hi) {
		return fmt.Errorf("since must precede until")
	}
	return nil
}
func rendered(e core.Event, q Query) (Event, bool) {
	v := Event{Seq: e.Seq, Time: time.UnixMilli(e.TimeMS).UTC(), Kind: e.Kind}
	if q.View == "raw" {
		if e.Kind != "raw" || len(e.Raw) == 0 {
			return Event{}, false
		}
		if json.Valid(e.Raw) {
			v.Raw = json.RawMessage(e.Raw)
		} else {
			v.RawBase64 = e.Raw
		}
		return v, true
	}
	switch e.Kind {
	case "input", "assistant":
		v.Text = e.Text
	case "tool_call", "tool_output", "tool_result":
		if !q.IncludeTools {
			return Event{}, false
		}
		v.Text = e.Text
		v.Payload = e.Payload
		v.RequestID = e.RequestID
	default:
		return Event{}, false
	}
	return v, true
}
func cursor(q Query, op string, last uint64) string {
	q.Cursor = ""
	q.Output = ""
	q.MaxBytes = 0
	b, _ := json.Marshal(continuation{1, op, q, last})
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Store) Search(ctx context.Context, q Query) (Reply, error) { return s.run(ctx, q, true) }
func (s *Store) Read(ctx context.Context, q Query) (Reply, error)   { return s.run(ctx, q, false) }
func (s *Store) run(ctx context.Context, q Query, search bool) (Reply, error) {
	op := "read"
	if search {
		op = "search"
	}
	var last uint64
	if q.Cursor != "" {
		if len(q.Cursor) > 32768 {
			return Reply{}, fmt.Errorf("cursor too large")
		}
		b, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		var c continuation
		if err != nil || json.Unmarshal(b, &c) != nil || c.Version != 1 || c.Operation != op || c.Query.Cursor != "" || c.Query.SnapshotSeq == nil {
			return Reply{}, fmt.Errorf("invalid %s cursor", op)
		}
		if q.Session != "" && q.Session != c.Query.Session {
			return Reply{}, fmt.Errorf("cursor belongs to a different session; use its UUID")
		}
		c.Query.Output = q.Output
		c.Query.MaxBytes = q.MaxBytes
		if q.Limit != 0 {
			c.Query.Limit = q.Limit
		}
		q = c.Query
		last = c.Last
	}
	now := s.now()
	if err := normalize(&q, now); err != nil {
		return Reply{}, err
	}
	if q.Output == "" {
		q.Output = "inline"
	}
	if q.Output != "inline" && q.Output != "file" {
		return Reply{}, fmt.Errorf("output must be inline or file")
	}
	if search && q.Output != "inline" {
		return Reply{}, fmt.Errorf("search returns metadata only; use conversation_read for file output")
	}
	if search && (q.LineStart > 0 || q.LineEnd > 0 || q.Before > 0 || q.After > 0) {
		return Reply{}, fmt.Errorf("line/context selection belongs to conversation_read")
	}
	budget := DefaultBytes
	maximum := MaxInlineBytes
	if q.Output == "file" {
		budget = MaxFileBytes
		maximum = MaxFileBytes
	}
	if q.MaxBytes != 0 {
		budget = q.MaxBytes
	}
	if budget < 256 || budget > maximum {
		return Reply{}, fmt.Errorf("max_bytes must be 256..%d", maximum)
	}
	var re *regexp.Regexp
	if q.Match == "regex" {
		pattern := q.Query
		if q.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return Reply{}, fmt.Errorf("invalid RE2 expression: %w", err)
		}
	}
	v, err := s.resolve(ctx, q.Session)
	if err != nil {
		return Reply{}, err
	}
	q.Session = v.ID
	events, err := s.journal(ctx, v)
	if err != nil {
		return Reply{}, err
	}
	r := details(v, events, now)
	r.View = q.View
	r.Since = q.Since
	r.Until = q.Until
	if q.SnapshotSeq == nil {
		q.SnapshotSeq = &r.SnapshotSeq
	} else {
		if *q.SnapshotSeq > r.SnapshotSeq {
			return Reply{}, fmt.Errorf("snapshot is ahead of retained journal")
		}
		r.SnapshotSeq = *q.SnapshotSeq
	}
	if last > r.SnapshotSeq {
		return Reply{}, fmt.Errorf("cursor is ahead of snapshot")
	}
	selected := map[uint64]bool{}
	exists := map[uint64]bool{}
	var candidates []Event
	for _, e := range events {
		if e.Seq > r.SnapshotSeq {
			break
		}
		exists[e.Seq] = true
		if out, ok := rendered(e, q); ok {
			candidates = append(candidates, out)
		}
	}
	for _, seq := range q.Seqs {
		if seq == 0 {
			return Reply{}, fmt.Errorf("seq must be positive")
		}
		idx := sort.Search(len(candidates), func(i int) bool { return candidates[i].Seq >= seq })
		found := idx < len(candidates) && candidates[idx].Seq == seq
		if !exists[seq] || seq <= r.TrimmedThrough {
			r.MissingSeqs = append(r.MissingSeqs, seq)
		} else if !found {
			r.UnavailableSeqs = append(r.UnavailableSeqs, seq)
		}
		if found {
			for i := max(0, idx-q.Before); i < min(len(candidates), idx+q.After+1); i++ {
				selected[candidates[i].Seq] = true
			}
		}
	}
	lo, _ := time.Parse(time.RFC3339Nano, q.Since)
	hi, _ := time.Parse(time.RFC3339Nano, q.Until)
	used := 0
	count := 0
	for _, e := range candidates {
		if err := ctx.Err(); err != nil {
			return Reply{}, err
		}
		if e.Seq <= last || (len(q.Seqs) > 0 && !selected[e.Seq]) || (!lo.IsZero() && e.Time.Before(lo)) || (!hi.IsZero() && !e.Time.Before(hi)) {
			continue
		}
		full, _ := json.Marshal(e)
		target := e.Text
		if q.View == "raw" {
			target = string(e.Raw)
			if e.Raw == nil {
				target = string(e.RawBase64)
			}
		} else if e.Payload != nil {
			target += "\n" + string(e.Payload)
		}
		matched := false
		if re != nil {
			matched = re.MatchString(target)
		} else {
			query := q.Query
			if q.IgnoreCase {
				target = strings.ToLower(target)
				query = strings.ToLower(query)
			}
			matched = strings.Contains(target, query)
		}
		if !matched {
			continue
		}
		if q.LineStart > 0 || q.LineEnd > 0 {
			lines := strings.Split(e.Text, "\n")
			start := max(1, q.LineStart)
			end := len(lines)
			if q.LineEnd > 0 {
				end = min(end, q.LineEnd)
			}
			if start > end {
				return Reply{}, fmt.Errorf("line range outside message")
			}
			e.Text = strings.Join(lines[start-1:end], "\n")
			e.LineStart = start
			e.LineEnd = end
		}
		body, _ := json.Marshal(e)
		cost := len(body)
		if q.Output == "file" {
			cost++
		}
		if count >= q.Limit || (!search && used+cost > budget) {
			r.HasMore = true
			r.NextCursor = cursor(q, op, last)
			if count == 0 {
				r.OversizedSeqs = []uint64{e.Seq}
				r.Message = "Event exceeds body budget; request output=file, a larger max_bytes, or a message line range."
			}
			break
		}
		count++
		last = e.Seq
		if search {
			r.Hits = append(r.Hits, Hit{e.Seq, e.Time, len(full)})
			r.ReturnedBytes += len(full)
		} else {
			r.Bodies = append(r.Bodies, e)
			r.ReturnedBytes += len(body)
			used += cost
		}
	}
	r.EventCount = count
	if !search && q.Output == "file" {
		if err = s.export(ctx, q, &r); err != nil {
			return Reply{}, err
		}
	}
	return r, nil
}
