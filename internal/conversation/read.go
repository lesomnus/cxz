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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// MaxRecordBytes bounds one committed record. A record this large is a tool
// result that ran away rather than something anybody wrote, and holding it to
// answer a page would cost more memory than the page is worth. Exceeding it is
// not a failure of the read: the record is skipped and named.
const MaxRecordBytes = 32 << 20

// MaxScanBytes bounds the whole journal a query will walk.
const MaxScanBytes = 256 << 20

// recordHeadBytes is how much of a skipped record is kept to identify it. seq
// is the third field core.Event declares, ahead of every field that can be
// large, so this is generous by three orders of magnitude.
const recordHeadBytes = 4 << 10

// unreadable is a committed record this reader would not hold. Seq is zero when
// the retained head did not name one, and After is the last sequence read
// before it, so the record is locatable either way.
type unreadable struct {
	Seq   uint64
	After uint64
	Bytes int
}

// Read committed records only. os.Root prevents a session symlink escaping the
// project; explicit limits also bound malformed/oversized journals before pruning.
//
// A record over MaxRecordBytes is skipped rather than ending the walk. The same
// defect was fixed once already in internal/supervisor: a bufio.Scanner cannot
// resume past an over-long token, so one runaway record used to discard every
// record read before it and answer nothing about the session at all. A committed
// journal is newline delimited, so the record after the skipped one is intact --
// which is why this side continues where the supervisor, reading a live stream,
// has to stop.
func (s *Store) journal(ctx context.Context, v Session) ([]core.Event, []unreadable, error) {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	dir := filepath.Join("sessions", v.RuntimeID)
	b, err := root.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return nil, nil, err
	}
	var manifest core.Session
	if json.Unmarshal(b, &manifest) != nil || manifest.ID != v.RuntimeID || manifest.ProjectID != s.Project {
		return nil, nil, fmt.Errorf("session manifest outside project scope")
	}
	f, err := root.Open(filepath.Join(dir, "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	var out []core.Event
	var skipped []unreadable
	total := 0
	var last uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		record, oversize, complete, readErr := nextRecord(r, s.limit(), recordHeadBytes)
		if !complete {
			// Either the end of the file, or a record without its newline --
			// the tail a supervisor is still writing. Neither is a committed
			// record, so neither is read, and an unterminated one is not
			// reported as skipped either, whatever its size.
			if readErr == nil || readErr == io.EOF {
				return out, skipped, nil
			}
			return nil, nil, readErr
		}
		size := oversize
		if size == 0 {
			size = len(record)
		}
		total += size
		if total > MaxScanBytes {
			return nil, nil, fmt.Errorf("journal exceeds %d MiB query scan limit", MaxScanBytes>>20)
		}
		if oversize > 0 {
			skipped = append(skipped, unreadable{Seq: recordSeq(record), After: last, Bytes: oversize})
		} else {
			line := bytes.TrimRight(record, "\r\n")
			var batch []core.Event
			if len(line) > 0 && line[0] == '[' {
				err = json.Unmarshal(line, &batch)
			} else {
				var e core.Event
				err = json.Unmarshal(line, &e)
				batch = []core.Event{e}
			}
			if err != nil {
				return nil, nil, fmt.Errorf("invalid committed journal record")
			}
			for _, e := range batch {
				if e.Seq <= last || (e.SessionID != "" && e.SessionID != v.RuntimeID) {
					return nil, nil, fmt.Errorf("invalid journal identity or sequence")
				}
				last = e.Seq
				out = append(out, e)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return out, skipped, nil
			}
			return nil, nil, readErr
		}
	}
}

// nextRecord reads through the next newline. Under limit it returns the record
// and a zero size. Over it, it returns the record's first keep bytes and the
// record's whole size, having consumed the rest -- so the caller can say which
// record it was and carry on with the next one.
//
// complete reports that a newline terminated the record. Without one there is
// nothing to carry on to: the bytes are a record still being written, and this
// reader only reads committed ones.
func nextRecord(r *bufio.Reader, limit, keep int) ([]byte, int, bool, error) {
	var record, head []byte
	size, over := 0, false
	for {
		chunk, err := r.ReadSlice('\n')
		if n := keep - len(head); n > 0 {
			// ReadSlice aliases the reader's buffer, so what is kept is copied.
			head = append(head, chunk[:min(n, len(chunk))]...)
		}
		size += len(chunk)
		if size > limit && !over {
			// Release what was accumulated; only the head is needed from here.
			over, record = true, nil
		}
		if !over {
			record = append(record, chunk...)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		complete := bytes.HasSuffix(chunk, newline)
		if over {
			return head, size, complete, err
		}
		return record, 0, complete, err
	}
}

var newline = []byte{'\n'}

// recordSeq reads the sequence out of the start of a record.
//
// A pattern rather than a parser, because the input is a fragment of a JSON
// document by construction and no parser accepts one. It cannot be fooled by a
// "seq" inside a payload: core.Event declares seq third, ahead of text, payload
// and raw, and the two fields before it hold a 24-hex session id and a run id.
// The first match in the head is therefore the record's own sequence, and a
// batch's is its first element's, which still locates it.
//
// Zero means the head named none, which the caller reports rather than guesses
// around.
func recordSeq(head []byte) uint64 {
	m := seqPattern.FindSubmatch(head)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseUint(string(m[1]), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

var seqPattern = regexp.MustCompile(`"seq"\s*:\s*([0-9]{1,19})`)

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
	events, skipped, err := s.journal(ctx, v)
	if err != nil {
		return Reply{}, err
	}
	r := details(v, events, s.now())
	noteUnreadable(&r, skipped)
	return r, nil
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
	events, skipped, err := s.journal(ctx, v)
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
	// Last, so it is not overwritten by the body-budget message: both can be
	// true of one reply, and this one is about the journal rather than the page.
	noteUnreadable(&r, skipped)
	if !search && q.Output == "file" {
		if err = s.export(ctx, q, &r); err != nil {
			return Reply{}, err
		}
	}
	return r, nil
}

// noteUnreadable names the records the journal would not hand over. It is not
// OversizedSeqs: that one means an event did not fit this page's byte budget,
// and its remedies -- output=file, a larger max_bytes, a line range -- do
// nothing here. A caller told to retry with a file would fail the same way.
func noteUnreadable(r *Reply, skipped []unreadable) {
	if len(skipped) == 0 {
		return
	}
	largest := 0
	unidentified := 0
	for _, v := range skipped {
		largest = max(largest, v.Bytes)
		if v.Seq == 0 {
			unidentified++
			continue
		}
		r.UnreadableSeqs = append(r.UnreadableSeqs, v.Seq)
	}
	note := fmt.Sprintf("%d committed record(s) exceed the %d MiB record limit and were skipped; the largest is %d MiB. Everything else in this journal was read.",
		len(skipped), MaxRecordBytes>>20, largest>>20)
	if len(skipped) == 1 {
		where := fmt.Sprintf("seq %d", skipped[0].Seq)
		if skipped[0].Seq == 0 {
			where = fmt.Sprintf("the record after seq %d", skipped[0].After)
		}
		note = fmt.Sprintf("%s is %d MiB, over the %d MiB record limit, and was skipped. Everything else in this journal was read.",
			where, skipped[0].Bytes>>20, MaxRecordBytes>>20)
	} else if unidentified > 0 {
		note += fmt.Sprintf(" %d could not be identified.", unidentified)
	}
	if r.Message == "" {
		r.Message = note
		return
	}
	r.Message += " " + note
}
