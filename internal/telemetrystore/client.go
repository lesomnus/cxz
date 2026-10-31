// Package telemetrystore queries the installation-private ClickHouse database.
package telemetrystore

import (
	"context"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	Endpoint, Password string
	HTTP               *http.Client
	RetentionDays      int
}

func New(endpoint, password string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.IdleConnTimeout = 30 * time.Second
	transport.MaxIdleConnsPerHost = 4
	return &Client{Endpoint: endpoint, Password: password, HTTP: &http.Client{Timeout: 20 * time.Second, Transport: transport}, RetentionDays: 90}
}

// Query uses ClickHouse typed parameters, never interpolation of caller input.
// Server-side limits bound analytics independently of database retention.
func (c *Client) Query(ctx context.Context, sql string, params map[string]string) ([]byte, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("database", "telemetry")
	q.Set("max_execution_time", "15")
	q.Set("max_result_bytes", "8388608")
	q.Set("max_memory_usage", "268435456")
	for k, v := range params {
		q.Set("param_"+k, v)
	}
	u.RawQuery = q.Encode()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(sql))
	if err != nil {
		return nil, err
	}
	r.SetBasicAuth("cxz", c.Password)
	r.Header.Set("Content-Type", "text/plain")
	res, err := c.HTTP.Do(r)
	if err != nil {
		return nil, fmt.Errorf("telemetry database unavailable: %w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 8<<20 {
		return nil, fmt.Errorf("telemetry query result exceeds 8 MiB")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telemetry database query failed (%d): %.500s", res.StatusCode, b)
	}
	return b, nil
}

// Pinned exporter 0.164.1 DDL, Apache-2.0, with nanosecond DateTime64 columns.
// Keeping precision here makes retries deduplicable and time-range APIs exact.
//
//go:embed schema/*.sql
var schema embed.FS

func (c *Client) Initialize(ctx context.Context, days int) error {
	for _, sql := range []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS telemetry.erased_sessions (session String, erased_at DateTime DEFAULT now()) ENGINE=ReplacingMergeTree ORDER BY session TTL erased_at + INTERVAL %d DAY`, days+7),
		`CREATE TABLE IF NOT EXISTS telemetry.collection (id UInt8, started DateTime64(3)) ENGINE=ReplacingMergeTree ORDER BY id`,
		`INSERT INTO telemetry.collection SELECT 1,now64(3) WHERE NOT EXISTS (SELECT 1 FROM telemetry.collection WHERE id=1)`,
		fmt.Sprintf(`ALTER TABLE telemetry.erased_sessions MODIFY TTL erased_at + INTERVAL %d DAY`, days+7),
	} {
		if _, err := c.Query(ctx, sql, nil); err != nil {
			return err
		}
	}
	for _, kind := range []string{"sum", "gauge", "histogram", "exp_histogram", "summary"} {
		template, err := schema.ReadFile("schema/" + kind + ".sql")
		if err != nil {
			return err
		}
		ttl := fmt.Sprintf("TTL toDateTime(TimeUnix) + INTERVAL %d DAY", days)
		sql := fmt.Sprintf(string(template), "telemetry", "otel_metrics_"+kind, "", "MergeTree()", ttl)
		if _, err = c.Query(ctx, sql, nil); err != nil {
			return err
		}
		if _, err = c.Query(ctx, "ALTER TABLE telemetry.otel_metrics_"+kind+" MODIFY "+ttl, nil); err != nil {
			return err
		}
	}
	return nil
}

// Erasure is a durable query exclusion before an asynchronous physical delete.
// Late batches from the Collector queue cannot resurrect an erased session.
func (c *Client) Erase(ctx context.Context, session string) error {
	return c.EraseMany(ctx, []string{session})
}
func (c *Client) EraseMany(ctx context.Context, sessions []string) error {
	if len(sessions) == 0 {
		return nil
	}
	for _, id := range sessions {
		b, err := hex.DecodeString(id)
		if err != nil || len(b) != 16 {
			return fmt.Errorf("invalid telemetry erasure identity")
		}
	}
	_, err := c.Query(ctx, `INSERT INTO telemetry.erased_sessions(session) SELECT arrayJoin({sessions:Array(String)})`, map[string]string{"sessions": "['" + strings.Join(sessions, "','") + "']"})
	if err != nil {
		return err
	}
	return c.CleanErased(ctx)
}
func (c *Client) CleanErased(ctx context.Context) error {
	b, err := c.Query(ctx, "SELECT count() FROM telemetry.erased_sessions", nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) == "0" {
		return nil
	}
	for _, table := range []string{"sum", "gauge", "histogram", "exp_histogram", "summary"} {
		_, err := c.Query(ctx, `ALTER TABLE telemetry.otel_metrics_`+table+` DELETE WHERE ResourceAttributes['cxz.session.id'] IN (SELECT session FROM telemetry.erased_sessions)`, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
