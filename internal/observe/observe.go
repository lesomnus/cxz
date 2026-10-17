// Package observe keeps what the manager has been doing: a bounded ring of
// finished RPCs, per-method aggregates, and a file of the ones that failed or
// ran long.
//
// Metadata only -- method, status, elapsed, message sizes -- never a request or
// a reply. This is read by clients over the plaintext surface and written to a
// file on disk, and neither is a place for session content.
//
// It exists because the manager's RPC log was already the answer to every
// question asked of a stalled installation -- which method, which status, how
// long -- and it was only ever on the manager's stdout, reachable with
// `docker logs` from the host and nowhere else. See cxz#211.
package observe

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (
	// Ring is how many finished RPCs are kept in memory. At roughly ten calls
	// a second, the quiet polling of an idle installation included, this is
	// most of an hour -- long enough that a question asked after the fact
	// still has an answer.
	Ring = 4096

	// Slow is when a unary call is worth keeping past the ring. Streams are
	// long by design and are never slow.
	Slow = 2 * time.Second

	fileLimit = 4 << 20
)

// Record is one finished RPC.
type Record struct {
	At        time.Time `json:"at"`
	Method    string    `json:"method"`
	Code      string    `json:"code"`
	ElapsedMs int64     `json:"elapsed_ms"`
	WaitedMs  int64     `json:"waited_ms,omitempty"`
	WaitedOn  string    `json:"waited_on,omitempty"`
	Stream    bool      `json:"stream,omitempty"`
}

// Stat is one method's shape. P50 and P99 are histogram bucket bounds, so they
// read as "at or under", which is what a bucketed histogram can honestly say.
type Stat struct {
	Method   string `json:"method"`
	Calls    int64  `json:"calls"`
	Failed   int64  `json:"failed"`
	InFlight int64  `json:"in_flight"`
	P50Ms    int64  `json:"p50_ms"`
	P99Ms    int64  `json:"p99_ms"`
	MaxMs    int64  `json:"max_ms"`
}

var buckets = []time.Duration{
	time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond,
	10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond,
	100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond,
	time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second,
	30 * time.Second, time.Minute, 5 * time.Minute,
}

type stat struct {
	calls, failed, inFlight int64
	max                     time.Duration
	counts                  []int64
}

type Log struct {
	path string
	on   bool

	mu      sync.Mutex
	ring    []Record
	next    int
	wrapped bool
	methods map[string]*stat
	verbose time.Time
	file    *os.File
	written int64
}

func New(root string) *Log {
	return &Log{path: Path(root), on: root != "", ring: make([]Record, Ring), methods: map[string]*stat{}}
}

// File is where the kept records are appended, on the manager's filesystem.
func (l *Log) File() string { return l.path }

type waitKey struct{}
type wait struct {
	mu sync.Mutex
	on string
	d  time.Duration
}

// Waited annotates the call in flight with time it spent waiting rather than
// working. A call that took thirty seconds says nothing on its own; one that
// waited twenty-eight of them on the update gate says everything.
func Waited(ctx context.Context, on string, d time.Duration) {
	w, _ := ctx.Value(waitKey{}).(*wait)
	if w == nil || d <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.d += d
	if w.on == "" {
		w.on = on
	}
}

// Wait runs f and attributes its duration to on.
func Wait(ctx context.Context, on string, f func()) {
	start := time.Now()
	f()
	Waited(ctx, on, time.Since(start))
}

func (l *Log) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		w := &wait{}
		start := time.Now()
		l.begin(info.FullMethod)
		res, err := next(context.WithValue(ctx, waitKey{}, w), req)
		l.end(info.FullMethod, start, err, w, false)
		return res, err
	}
}

type observedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s observedStream) Context() context.Context { return s.ctx }

func (l *Log) Stream() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		w := &wait{}
		start := time.Now()
		l.begin(info.FullMethod)
		err := next(srv, observedStream{stream, context.WithValue(stream.Context(), waitKey{}, w)})
		l.end(info.FullMethod, start, err, w, true)
		return err
	}
}

func (l *Log) begin(method string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stat(method).inFlight++
}
func (l *Log) end(method string, start time.Time, err error, w *wait, stream bool) {
	elapsed := time.Since(start)
	w.mu.Lock()
	waited, on := w.d, w.on
	w.mu.Unlock()
	code := status.Code(err)
	r := Record{
		At: start, Method: method, Code: code.String(), ElapsedMs: elapsed.Milliseconds(),
		WaitedMs: waited.Milliseconds(), WaitedOn: on, Stream: stream,
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.stat(method)
	s.inFlight--
	s.calls++
	if code != 0 {
		s.failed++
	}
	if elapsed > s.max {
		s.max = elapsed
	}
	s.counts[bucket(elapsed)]++
	l.ring[l.next] = r
	l.next++
	if l.next == len(l.ring) {
		l.next, l.wrapped = 0, true
	}
	if code != 0 || (!stream && elapsed >= Slow) || time.Now().Before(l.verbose) {
		l.persist(r)
	}
}
func (l *Log) stat(method string) *stat {
	s := l.methods[method]
	if s == nil {
		s = &stat{counts: make([]int64, len(buckets)+1)}
		l.methods[method] = s
	}
	return s
}
func bucket(d time.Duration) int {
	for i, b := range buckets {
		if d <= b {
			return i
		}
	}
	return len(buckets)
}

// Path is the file the records that outlive memory are appended to.
func Path(root string) string { return filepath.Join(root, "logs", "rpc.jsonl") }

// persist appends one record, rotating at fileLimit and keeping one previous
// file. Called with the lock held; a file that cannot be written is given up
// on rather than retried, because observation must not become the thing that
// fails an RPC.
func (l *Log) persist(r Record) {
	if !l.on {
		return
	}
	if l.file == nil {
		if e := os.MkdirAll(filepath.Dir(l.path), 0700); e != nil {
			l.on = false
			return
		}
		f, e := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			l.on = false
			return
		}
		if st, e := f.Stat(); e == nil {
			l.written = st.Size()
		}
		l.file = f
	}
	b, e := json.Marshal(r)
	if e != nil {
		return
	}
	b = append(b, '\n')
	n, e := l.file.Write(b)
	l.written += int64(n)
	if e != nil {
		l.file.Close()
		l.file, l.on = nil, false
		return
	}
	if l.written < fileLimit {
		return
	}
	l.file.Close()
	l.file = nil
	l.written = 0
	if e = os.Rename(l.path, l.path+".1"); e != nil {
		l.on = false
	}
}

// Records returns the kept RPCs, oldest first, optionally narrowed to one
// method and to what happened since a point in time.
func (l *Log) Records(since time.Time, method string, limit int) []Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Record
	for i := 0; i < len(l.ring); i++ {
		idx := (l.next + i) % len(l.ring)
		r := l.ring[idx]
		if r.Method == "" {
			continue
		}
		if method != "" && r.Method != method {
			continue
		}
		if !since.IsZero() && r.At.Before(since) {
			continue
		}
		out = append(out, r)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Stats returns one entry per method seen, busiest first.
func (l *Log) Stats() []Stat {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Stat, 0, len(l.methods))
	for method, s := range l.methods {
		out = append(out, Stat{
			Method: method, Calls: s.calls, Failed: s.failed, InFlight: s.inFlight,
			P50Ms: quantile(s, 0.50), P99Ms: quantile(s, 0.99), MaxMs: s.max.Milliseconds(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Method < out[j].Method
	})
	return out
}
func quantile(s *stat, q float64) int64 {
	if s.calls == 0 {
		return 0
	}
	want, seen := int64(q*float64(s.calls)), int64(0)
	for i, n := range s.counts {
		seen += n
		if seen > want {
			if i == len(buckets) {
				return s.max.Milliseconds()
			}
			return buckets[i].Milliseconds()
		}
	}
	return s.max.Milliseconds()
}

// Capture writes every record, not only the failed and the slow ones, until it
// expires. It expires on its own because a verbose setting that has to be
// turned off is one that is left on.
func (l *Log) Capture(d time.Duration) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if d <= 0 {
		l.verbose = time.Time{}
		return time.Time{}
	}
	l.verbose = time.Now().Add(d)
	return l.verbose
}
func (l *Log) CapturingUntil() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().After(l.verbose) {
		return time.Time{}
	}
	return l.verbose
}
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	f := l.file
	l.file = nil
	return f.Close()
}
