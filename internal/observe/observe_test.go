package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func call(t *testing.T, l *Log, method string, err error, work func(context.Context)) {
	t.Helper()
	_, got := l.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(ctx context.Context, _ any) (any, error) {
			if work != nil {
				work(ctx)
			}
			return nil, err
		})
	if (got == nil) != (err == nil) {
		t.Fatal("interceptor changed the result", got)
	}
}

func lines(t *testing.T, root string) []Record {
	t.Helper()
	f, e := os.Open(Path(root))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	var out []Record
	s := bufio.NewScanner(f)
	for s.Scan() {
		var r Record
		if e := json.Unmarshal(s.Bytes(), &r); e != nil {
			t.Fatal(e)
		}
		out = append(out, r)
	}
	return out
}

// Only the failed and the slow outlive memory; the quiet polling of an idle
// installation must not fill a disk.
func TestOnlyFailedAndSlowArePersisted(t *testing.T) {
	root := t.TempDir()
	l := New(root)
	defer l.Close()
	call(t, l, "/cxz.SessionService/Get", nil, nil)
	call(t, l, "/cxz.SessionService/Send", status.Error(codes.DeadlineExceeded, "no"), nil)
	if got := lines(t, root); len(got) != 1 || got[0].Method != "/cxz.SessionService/Send" || got[0].Code != "DeadlineExceeded" {
		t.Fatal("persisted the wrong records", got)
	}
	if n := len(l.Records(time.Time{}, "", 0)); n != 2 {
		t.Fatal("ring did not keep both", n)
	}
	if n := len(l.Records(time.Time{}, "/cxz.SessionService/Get", 0)); n != 1 {
		t.Fatal("method filter", n)
	}
	if n := len(l.Records(time.Now().Add(time.Minute), "", 0)); n != 0 {
		t.Fatal("since filter", n)
	}
}

func TestCaptureWritesEverythingUntilItExpires(t *testing.T) {
	root := t.TempDir()
	l := New(root)
	defer l.Close()
	if !l.CapturingUntil().IsZero() {
		t.Fatal("capturing without being asked")
	}
	until := l.Capture(time.Minute)
	if until.IsZero() || l.CapturingUntil().IsZero() {
		t.Fatal("capture did not start")
	}
	call(t, l, "/cxz.SessionService/Get", nil, nil)
	if got := lines(t, root); len(got) != 1 {
		t.Fatal("capture did not persist an ordinary call", got)
	}
	l.Capture(0)
	if !l.CapturingUntil().IsZero() {
		t.Fatal("capture did not stop")
	}
	call(t, l, "/cxz.SessionService/Get", nil, nil)
	if got := lines(t, root); len(got) != 1 {
		t.Fatal("capture kept writing after it was cleared", got)
	}
}

// A call that took thirty seconds says nothing; one that waited on the update
// gate for twenty-eight of them says everything.
func TestWaitIsAttributedToTheCallInFlight(t *testing.T) {
	l := New("")
	call(t, l, "/cxz.SessionService/Send", nil, func(ctx context.Context) {
		Wait(ctx, "update gate", func() { time.Sleep(10 * time.Millisecond) })
		Waited(ctx, "ignored, the first wait names the record", time.Millisecond)
	})
	got := l.Records(time.Time{}, "", 0)
	if len(got) != 1 || got[0].WaitedOn != "update gate" || got[0].WaitedMs < 10 {
		t.Fatal("wait not attributed", got)
	}
	// A context without a call in flight is not an error; nothing records it.
	Waited(context.Background(), "nowhere", time.Second)
}

func TestRingWrapsAndKeepsTheNewest(t *testing.T) {
	l := New("")
	for i := 0; i < Ring+10; i++ {
		call(t, l, fmt.Sprintf("/m/%d", i), nil, nil)
	}
	got := l.Records(time.Time{}, "", 0)
	if len(got) != Ring {
		t.Fatal("ring size", len(got))
	}
	if got[0].Method != "/m/10" || got[len(got)-1].Method != fmt.Sprintf("/m/%d", Ring+9) {
		t.Fatal("ring did not keep the newest, oldest first", got[0].Method, got[len(got)-1].Method)
	}
	if got := l.Records(time.Time{}, "", 3); len(got) != 3 || got[2].Method != fmt.Sprintf("/m/%d", Ring+9) {
		t.Fatal("limit did not take the newest", got)
	}
}

func TestStatsCountFailuresAndReportBuckets(t *testing.T) {
	l := New("")
	for i := 0; i < 99; i++ {
		call(t, l, "/m/a", nil, nil)
	}
	call(t, l, "/m/a", nil, func(context.Context) { time.Sleep(30 * time.Millisecond) })
	call(t, l, "/m/b", status.Error(codes.Unavailable, "no"), nil)
	all := l.Stats()
	if len(all) != 2 || all[0].Method != "/m/a" || all[0].Calls != 100 {
		t.Fatal("stats not busiest first", all)
	}
	if all[0].Failed != 0 || all[1].Failed != 1 {
		t.Fatal("failures not counted", all)
	}
	if all[0].InFlight != 0 {
		t.Fatal("finished call still in flight", all[0])
	}
	if all[0].P50Ms > 5 || all[0].P99Ms < 25 || all[0].MaxMs < 25 {
		t.Fatal("one slow call in a hundred should move p99, not p50", all[0])
	}
}

func TestStreamsAreNeverSlowAndAreMarked(t *testing.T) {
	root := t.TempDir()
	l := New(root)
	defer l.Close()
	e := l.Stream()(nil, nopStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: "/cxz.SessionService/Watch"},
		func(_ any, s grpc.ServerStream) error {
			Waited(s.Context(), "projection", 5*time.Millisecond)
			return nil
		})
	if e != nil {
		t.Fatal(e)
	}
	got := l.Records(time.Time{}, "", 0)
	if len(got) != 1 || !got[0].Stream || got[0].WaitedOn != "projection" {
		t.Fatal("stream record", got)
	}
	if n := lines(t, root); len(n) != 0 {
		t.Fatal("a stream that ended cleanly was persisted", n)
	}
}

type nopStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s nopStream) Context() context.Context { return s.ctx }

func TestFileIsBoundedByRotation(t *testing.T) {
	root := t.TempDir()
	l := New(root)
	defer l.Close()
	l.Capture(time.Minute)
	for i := 0; i < 40000; i++ {
		call(t, l, "/cxz.SessionService/"+strings.Repeat("m", 40), nil, nil)
	}
	st, e := os.Stat(Path(root))
	if e != nil {
		t.Fatal(e)
	}
	if st.Size() >= fileLimit {
		t.Fatal("file grew past the limit", st.Size())
	}
	if _, e = os.Stat(Path(root) + ".1"); e != nil {
		t.Fatal("rotation kept no previous file:", e)
	}
}
