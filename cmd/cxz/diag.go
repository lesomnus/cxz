//go:build !windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

// What the manager has been doing, read over the API rather than off the
// installation host's `docker logs`. See cxz#211.
func diagCommand() *xli.Command {
	parent := &xli.Command{Name: "diag", Brief: "What the manager has been doing: per-method aggregates and the calls it kept", Handler: onRun(func(_ context.Context, c *xli.Command) error {
		return c.PrintHelp(c.Writer)
	})}
	stats := &xli.Command{Name: "stats", Brief: "Per-method call counts, failures and durations", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		return withDiagnostics(ctx, c, func(ctx context.Context, client *resourceclient.Client) error {
			out, err := client.Diagnostics(ctx, "", time.Time{}, 0, true)
			if err != nil {
				return err
			}
			type row struct {
				Method   string `json:"method"`
				Calls    int64  `json:"calls"`
				Failed   int64  `json:"failed"`
				InFlight int64  `json:"in_flight"`
				P50Ms    int64  `json:"p50_ms"`
				P99Ms    int64  `json:"p99_ms"`
				MaxMs    int64  `json:"max_ms"`
			}
			rows := []row{}
			for _, s := range out.GetStats() {
				rows = append(rows, row{s.GetMethod(), s.GetCalls(), s.GetFailed(), s.GetInFlight(), s.GetP50Ms(), s.GetP99Ms(), s.GetMaxMs()})
			}
			return writeOutput(c, rows)
		})
	})}
	calls := &xli.Command{Name: "calls", Brief: "Recent calls, oldest first", Flags: flg.Flags{
		stringFlag("method", "Only this method, by its full gRPC name", ""),
		stringFlag("since", "Only what happened within this duration, such as 10m", ""),
		&flg.Uint64{Name: "limit", Default: ptr(uint64(100)), Brief: "Newest N, 1-4096"},
	}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		limit := flg.MustGet[uint64](c, "limit")
		if limit < 1 || limit > 4096 {
			return fmt.Errorf("limit must be 1-4096")
		}
		var since time.Time
		if v := flg.MustGet[string](c, "since"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}
			since = time.Now().Add(-d)
		}
		return withDiagnostics(ctx, c, func(ctx context.Context, client *resourceclient.Client) error {
			out, err := client.Diagnostics(ctx, flg.MustGet[string](c, "method"), since, int32(limit), false)
			if err != nil {
				return err
			}
			type row struct {
				At        string `json:"at"`
				Method    string `json:"method"`
				Code      string `json:"code"`
				ElapsedMs int64  `json:"elapsed_ms"`
				WaitedMs  int64  `json:"waited_ms"`
				WaitedOn  string `json:"waited_on"`
			}
			rows := []row{}
			for _, r := range out.GetRecords() {
				rows = append(rows, row{r.GetAt().AsTime().Local().Format("15:04:05.000"), r.GetMethod(), r.GetCode(), r.GetElapsedMs(), r.GetWaitedMs(), r.GetWaitedOn()})
			}
			return writeOutput(c, rows)
		})
	})}
	capture := &xli.Command{Name: "capture", Brief: "Keep every call for a while, not only the failed and the slow ones", Flags: flg.Flags{
		stringFlag("for", "How long, such as 15m; 0 stops an open window", "15m"),
	}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		d, err := time.ParseDuration(flg.MustGet[string](c, "for"))
		if err != nil {
			return err
		}
		if d < 0 || d > time.Hour {
			return fmt.Errorf("a capture window must be 0 to 1h")
		}
		return withDiagnostics(ctx, c, func(ctx context.Context, client *resourceclient.Client) error {
			out, err := client.CaptureDiagnostics(ctx, d)
			if err != nil {
				return err
			}
			until := "off"
			if out.HasCapturingUntil() {
				until = out.GetCapturingUntil().AsTime().Local().Format(time.TimeOnly)
			}
			return writeOutput(c, struct {
				CapturingUntil string `json:"capturing_until"`
				Path           string `json:"path"`
			}{until, out.GetPath()})
		})
	})}
	parent.Commands = xli.Commands{stats, calls, capture}
	return parent
}

func withDiagnostics(ctx context.Context, c *xli.Command, fn func(context.Context, *resourceclient.Client) error) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, err := server.Dial(stateFrom(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	defer client.Close()
	return fn(ctx, client)
}
