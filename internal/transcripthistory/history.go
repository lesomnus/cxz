// Package transcripthistory builds a disposable, indexed reading view of the
// native journal. No provider payloads or durable events are rewritten.
package transcripthistory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/shellview"
	"github.com/lesomnus/cxz/internal/toolview"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const DefaultPage = 256
const MaxPage = 1024

type Decode func([]byte) (*api.Event, error)
type Version struct {
	Seq, Updated uint64
	Event        *api.Event
}
type Projection struct {
	Versions  []Version
	Rows      map[uint64]*api.Event
	tools     map[string]*api.Event
	approvals map[string]string
	last      map[string]*api.Event
	input     map[string]int64
	inputSeq  map[string]uint64
}

func New() *Projection {
	return &Projection{Rows: map[uint64]*api.Event{}, tools: map[string]*api.Event{}, approvals: map[string]string{}, last: map[string]*api.Event{}, input: map[string]int64{}, inputSeq: map[string]uint64{}}
}
func identity(run, id string) string { return run + "\x00" + id }
func object(raw []byte) map[string]any {
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if p == nil {
		p = map[string]any{}
	}
	return p
}
func child(p map[string]any, key string) map[string]any {
	v, _ := p[key].(map[string]any)
	if v == nil {
		v = map[string]any{}
	}
	return v
}
func text(p map[string]any, key string) string { v, _ := p[key].(string); return v }
func related(e *api.Event) string {
	p := object(e.Payload)
	if e.Text == "AskUserQuestion" || e.Text == "item/tool/requestUserInput" {
		return ""
	}
	if id := text(p, "tool_use_id"); id != "" {
		return id
	}
	return text(child(p, "params"), "itemId")
}

var hidden = map[string]bool{"raw": true, "state": true, "vendor": true, "intent": true, "receipt": true, "approval_resolved": true, "models": true, "models_status": true, "usage": true, "usage_status": true, "history_checkpoint": true}

func (p *Projection) put(e *api.Event, updated uint64) {
	p.Rows[e.Seq] = e
	p.Versions = append(p.Versions, Version{e.Seq, updated, e})
}
func summary(e *api.Event) *api.Event {
	v := proto.Clone(e).(*api.Event)
	v.Payload = nil
	label := &api.ToolSummary{Name: e.Text, State: "pending"}
	data := object(e.Payload)
	item := child(data, "item")
	if text(item, "type") == "commandExecution" {
		label.Name = "Bash"
	} else if text(item, "type") == "fileChange" {
		label.Name = "Files"
	} else if text(item, "type") != "" {
		label.Name = text(item, "type")
	}
	input := data
	if v, ok := data["input"].(map[string]any); ok {
		input = v
	}
	command := text(item, "command")
	if command == "" {
		command = text(input, "command")
	}
	if command != "" {
		label.Shell, label.Command = shellview.Unwrap(command)
	} else {
		label.Command = text(input, "file_path")
		if changes, ok := item["changes"].([]any); ok {
			var files []string
			for _, change := range changes {
				if v, ok := change.(map[string]any); ok && text(v, "path") != "" {
					files = append(files, text(v, "path"))
				}
			}
			label.Command = strings.Join(files, ", ")
		}
	}
	if label.Name == "" {
		label.Name = "Tool"
	}
	provider := "claude"
	if text(item, "type") != "" {
		provider = "codex"
	}
	if activity, ok := toolview.ToolView(provider, e.Text, e.Payload); ok {
		const maxFiles = 256
		for _, f := range activity.Files[:min(len(activity.Files), maxFiles)] {
			label.Files = append(label.Files, &api.ToolFileSummary{Path: clip(f.Path, 1024), MovePath: clip(f.MovePath, 1024), Action: f.Action, Added: uint32(f.Added), Removed: uint32(f.Removed), Lines: uint32(f.Lines), Measure: f.Measure, PerMatch: f.PerMatch})
		}
		label.OmittedFiles = uint32(len(activity.Files) - len(label.Files))
	}
	if text(item, "status") == "inProgress" {
		label.State = "working"
	}
	// A one-line preview cannot be a megabyte-sized script or file body.
	label.Name = clip(label.Name, 160)
	label.Command = clip(label.Command, 1024)
	v.Text = label.Name
	v.ToolSummary = label
	return v
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func failed(e *api.Event) bool {
	p := object(e.Payload)
	item := child(p, "item")
	exit, _ := item["exitCode"].(float64)
	return p["is_error"] == true || exit != 0 || slices.Contains([]string{"failed", "denied", "declined", "canceled", "cancelled", "interrupted"}, text(item, "status"))
}
func (p *Projection) Apply(e *api.Event) {
	key := identity(e.RunId, e.RequestId)
	switch e.Kind {
	case "input":
		p.input[e.RunId] = e.TimeMs
		p.inputSeq[e.RunId] = e.Seq
		delete(p.last, e.RunId)
	case "assistant":
		p.last[e.RunId] = e
	case "tool_call", "tool_result", "tool_output":
		if e.RequestId != "" {
			v := p.tools[key]
			if v == nil || e.Kind == "tool_call" {
				v = summary(e)
			} else {
				v = proto.Clone(v).(*api.Event)
			}
			state := v.ToolSummary.State
			if e.Kind == "tool_result" {
				// Codex can announce an empty file-change list and report the paths/diff
				// only on completion. Enrich the original anchor instead of adding a row.
				result := summary(e).ToolSummary
				if text(child(object(e.Payload), "item"), "type") == "fileChange" && len(result.Files) > 0 {
					v.ToolSummary.Files = result.Files
					v.ToolSummary.OmittedFiles = result.OmittedFiles
					v.ToolSummary.Command = result.Command
				}
				v.ToolSummary.State = "completed"
				if failed(e) {
					v.ToolSummary.State = "failed"
				}
			} else if e.Kind == "tool_output" {
				if state == "completed" || state == "failed" {
					return
				}
				v.ToolSummary.State = "working"
				if p.tools[key] != nil && state == "working" {
					return
				}
			}
			p.tools[key] = v
			p.put(v, e.Seq)
			return
		}
	case "approval":
		if id := related(e); id != "" {
			toolKey := identity(e.RunId, id)
			p.approvals[key] = toolKey
			if previous := p.tools[toolKey]; previous != nil {
				v := proto.Clone(previous).(*api.Event)
				if v.ToolSummary.State != "completed" && v.ToolSummary.State != "failed" {
					v.ToolSummary.State = "pending"
					p.tools[toolKey] = v
					p.put(v, e.Seq)
				}
				return
			}
		}
	case "approval_resolved":
		if previous := p.tools[p.approvals[key]]; previous != nil {
			v := proto.Clone(previous).(*api.Event)
			if v.ToolSummary.State != "completed" && v.ToolSummary.State != "failed" {
				v.ToolSummary.State = "working"
				if e.Text != "allowed" {
					v.ToolSummary.State = "failed"
				}
				p.tools[p.approvals[key]] = v
				p.put(v, e.Seq)
			}
		}
		return
	case "turn_end":
		if e.Text == "completed" {
			var target *api.Event
			completion := e.GetResponse().GetCompletionJson()
			if len(completion) > 0 {
				var c struct {
					ResponseSeq uint64 `json:"response_seq,string"`
				}
				if json.Unmarshal(completion, &c) == nil {
					target = p.Rows[c.ResponseSeq]
				}
			} else {
				if last := p.last[e.RunId]; last != nil {
					target = p.Rows[last.Seq]
				}
				if target != nil && target.GetResponse().GetPhase() != "" && target.GetResponse().GetPhase() != "final_answer" {
					target = nil
				}
				if target != nil {
					payload := object(e.Payload)
					c := map[string]any{"response_seq": fmt.Sprint(target.Seq), "token_scope": "turn"}
					duration, ok := payload["duration_ms"].(float64)
					if !ok {
						duration, ok = child(payload, "turn")["durationMs"].(float64)
					}
					if ok && duration >= 0 {
						c["duration_ms"], c["duration_source"] = duration, "provider"
					} else if start := p.input[e.RunId]; start > 0 && e.TimeMs >= start {
						c["duration_ms"], c["duration_source"] = e.TimeMs-start, "cxz"
					}
					metrics := map[string]float64{}
					for native, common := range map[string]string{"input_tokens": "input_tokens", "output_tokens": "output_tokens", "cache_read_input_tokens": "cache_read_tokens", "cache_creation_input_tokens": "cache_write_tokens"} {
						if v, ok := child(payload, "usage")[native].(float64); ok && v >= 0 {
							metrics[common] = v
						}
					}
					for native, common := range map[string]string{"total_cost_usd": "cost_usd", "duration_api_ms": "api_duration_ms", "num_turns": "api_turns"} {
						if v, ok := payload[native].(float64); ok && v >= 0 {
							metrics[common] = v
						}
					}
					c["metrics"] = metrics
					completion, _ = json.Marshal(c)
				}
			}
			if target != nil && target.Kind == "assistant" && target.RunId == e.RunId && target.Seq < e.Seq && !slices.Contains([]string{"commentary", "subagent"}, target.GetResponse().GetPhase()) {
				v := proto.Clone(target).(*api.Event)
				if v.Response == nil {
					v.Response = &api.ResponseMetadata{}
				}
				v.Response.CompletionJson = completion
				p.put(v, e.Seq)
				delete(p.last, e.RunId)
				delete(p.input, e.RunId)
				return
			}
		}
		delete(p.last, e.RunId)
		delete(p.input, e.RunId)
	}
	if hidden[e.Kind] {
		return
	}
	v := proto.Clone(e).(*api.Event)
	// Questions retain their options; other native bodies are loaded on demand.
	if e.Kind != "approval" {
		v.Payload = nil
	}
	if e.Kind == "assistant" && v.Response == nil {
		phase := text(child(object(e.Payload), "item"), "phase")
		if phase != "" {
			v.Response = &api.ResponseMetadata{Phase: phase}
		}
	}
	p.put(v, e.Seq)
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS transcript_rows(session_id TEXT,seq INTEGER,updated INTEGER,kind TEXT,data BLOB,PRIMARY KEY(session_id,seq,updated))`,
	`CREATE INDEX IF NOT EXISTS transcript_kind ON transcript_rows(session_id,kind,seq,updated)`,
	`CREATE TABLE IF NOT EXISTS transcript_records(session_id TEXT,seq INTEGER,run_id TEXT,request_id TEXT,kind TEXT,related_id TEXT,PRIMARY KEY(session_id,seq))`,
	`CREATE INDEX IF NOT EXISTS transcript_execution ON transcript_records(session_id,run_id,request_id,seq)`,
	`CREATE INDEX IF NOT EXISTS transcript_related ON transcript_records(session_id,run_id,related_id,seq)`,
	`CREATE INDEX IF NOT EXISTS transcript_metadata ON transcript_records(session_id,kind,seq)`,
	`CREATE TABLE IF NOT EXISTS transcript_state(session_id TEXT PRIMARY KEY,count INTEGER,first INTEGER,last INTEGER)`,
	`CREATE TABLE IF NOT EXISTS transcript_format(session_id TEXT PRIMARY KEY,version INTEGER)`,
}

const projectionFormat = 2

// Sync indexes only native non-raw records. A cache backfill or retention change
// rebuilds this disposable session index; normal queries read only the new suffix.
func Sync(ctx context.Context, db *sql.DB, id string, decode Decode) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range schema {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	var count, first, last uint64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(seq),0),COALESCE(MAX(seq),0) FROM events WHERE session_id=?`, id).Scan(&count, &first, &last); err != nil {
		return err
	}
	var oldCount, oldFirst, oldLast uint64
	err = tx.QueryRowContext(ctx, `SELECT count,first,last FROM transcript_state WHERE session_id=?`, id).Scan(&oldCount, &oldFirst, &oldLast)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var format int
	err = tx.QueryRowContext(ctx, `SELECT version FROM transcript_format WHERE session_id=?`, id).Scan(&format)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if format == projectionFormat && oldCount == count && oldFirst == first && oldLast == last {
		return tx.Commit()
	}
	var suffix uint64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id=? AND seq>?`, id, oldLast).Scan(&suffix); err != nil {
		return err
	}
	rebuild := format != projectionFormat || oldCount == 0 || oldFirst != first || count != oldCount+suffix || last < oldLast
	if rebuild {
		oldLast = 0
		for _, table := range []string{"transcript_rows", "transcript_records"} {
			if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE session_id=?`, id); err != nil {
				return err
			}
		}
	}
	projection := New()
	if !rebuild {
		// Restoring the reduced tools/last responses avoids walking old native bodies.
		rows, e := tx.QueryContext(ctx, `SELECT data FROM transcript_rows r WHERE session_id=? AND updated=(SELECT MAX(updated) FROM transcript_rows x WHERE x.session_id=r.session_id AND x.seq=r.seq) ORDER BY seq`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var b []byte
			if e = rows.Scan(&b); e != nil {
				rows.Close()
				return e
			}
			var v api.Event
			if e = json.Unmarshal(b, &v); e != nil {
				rows.Close()
				return e
			}
			projection.Rows[v.Seq] = &v
			if v.ToolSummary != nil {
				projection.tools[identity(v.RunId, v.RequestId)] = &v
			}
			if v.Kind == "assistant" && len(v.GetResponse().GetCompletionJson()) == 0 {
				projection.last[v.RunId] = &v
			}
			if v.Kind == "input" {
				delete(projection.last, v.RunId)
				projection.input[v.RunId] = v.TimeMs
				projection.inputSeq[v.RunId] = v.Seq
			}
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		rows, e = tx.QueryContext(ctx, `SELECT run_id,MAX(seq) FROM transcript_records WHERE session_id=? AND kind='turn_end' GROUP BY run_id`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var run string
			var seq uint64
			if e = rows.Scan(&run, &seq); e != nil {
				rows.Close()
				return e
			}
			if v := projection.last[run]; v != nil && v.Seq <= seq {
				delete(projection.last, run)
			}
			if projection.inputSeq[run] <= seq {
				delete(projection.input, run)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = tx.QueryContext(ctx, `SELECT run_id,request_id,related_id FROM transcript_records WHERE session_id=? AND kind='approval' AND related_id!=''`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var run, request, related string
			if e = rows.Scan(&run, &request, &related); e != nil {
				rows.Close()
				return e
			}
			projection.approvals[identity(run, request)] = identity(run, related)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT data FROM events WHERE session_id=? AND seq>? AND json_extract(data,'$.kind')!='raw' ORDER BY seq`, id, oldLast)
	if err != nil {
		return err
	}
	var native []*api.Event
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		v, e := decode(b)
		if e != nil {
			rows.Close()
			return e
		}
		native = append(native, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range native {
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_records VALUES(?,?,?,?,?,?)`, id, e.Seq, e.RunId, e.RequestId, e.Kind, related(e)); err != nil {
			return err
		}
		projection.Apply(e)
	}
	for _, v := range projection.Versions {
		b, e := json.Marshal(v.Event)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_rows VALUES(?,?,?,?,?)`, id, v.Seq, v.Updated, v.Event.Kind, b); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_state VALUES(?,?,?,?)`, id, count, first, last); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_format VALUES(?,?)`, id, projectionFormat); err != nil {
		return err
	}
	return tx.Commit()
}

const currentVersion = ` updated<=? AND updated=(SELECT MAX(updated) FROM transcript_rows x WHERE x.session_id=r.session_id AND x.seq=r.seq AND x.updated<=?) `

func Page(ctx context.Context, db *sql.DB, r *api.TranscriptRequest, decode Decode) (*api.TranscriptReply, error) {
	if r.AfterSeq > 0 && r.BeforeSeq > 0 {
		return nil, status.Error(codes.InvalidArgument, "before_seq and after_seq are mutually exclusive")
	}
	out := &api.TranscriptReply{SnapshotSeq: r.SnapshotSeq}
	if out.SnapshotSeq == 0 {
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events WHERE session_id=?`, r.SessionId).Scan(&out.SnapshotSeq); err != nil {
			return nil, err
		}
	}
	limit := pageLimit(r.Limit)
	query := `SELECT data FROM transcript_rows r WHERE session_id=? AND ` + currentVersion
	args := []any{r.SessionId, out.SnapshotSeq, out.SnapshotSeq}
	if r.AfterSeq > 0 {
		query += ` AND seq>? ORDER BY seq ASC LIMIT ?`
		args = append(args, r.AfterSeq, limit)
	} else {
		if r.BeforeSeq > 0 {
			query += ` AND seq<?`
			args = append(args, r.BeforeSeq)
		}
		query += ` ORDER BY seq DESC LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return nil, err
		}
		var e api.Event
		if err = json.Unmarshal(b, &e); err != nil {
			rows.Close()
			return nil, err
		}
		out.Events = append(out.Events, &e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if r.AfterSeq == 0 {
		slices.Reverse(out.Events)
	}
	if len(out.Events) > 0 {
		first, last := out.Events[0].Seq, out.Events[len(out.Events)-1].Seq
		for _, q := range []struct {
			op   string
			seq  uint64
			dest *bool
		}{{"<", first, &out.HasOlder}, {">", last, &out.HasNewer}} {
			if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transcript_rows r WHERE session_id=? AND `+currentVersion+` AND seq`+q.op+`?)`, r.SessionId, out.SnapshotSeq, out.SnapshotSeq, q.seq).Scan(q.dest); err != nil {
				return nil, err
			}
		}
		var b []byte
		err = db.QueryRowContext(ctx, `SELECT data FROM transcript_rows r WHERE session_id=? AND kind='input' AND seq<? AND `+currentVersion+` ORDER BY seq DESC LIMIT 1`, r.SessionId, first, out.SnapshotSeq, out.SnapshotSeq).Scan(&b)
		if err == nil {
			var e api.Event
			if err = json.Unmarshal(b, &e); err != nil {
				return nil, err
			}
			out.PrecedingInput = &e
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}
	rows, err = db.QueryContext(ctx, `SELECT e.data FROM transcript_records r JOIN events e ON e.session_id=r.session_id AND e.seq=r.seq WHERE r.session_id=? AND r.seq<=? AND r.kind IN ('models','usage','compact','turn_end') ORDER BY r.seq DESC LIMIT 64`, r.SessionId, out.SnapshotSeq)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return nil, err
		}
		e, x := decode(b)
		if x != nil {
			rows.Close()
			return nil, x
		}
		if v := metadata(e); v != nil {
			out.Metadata = append(out.Metadata, v)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	slices.Reverse(out.Metadata)
	return out, nil
}
func Details(ctx context.Context, db *sql.DB, r *api.EventDetailsRequest, decode Decode) (*api.EventBatch, error) {
	var b []byte
	if err := db.QueryRowContext(ctx, `SELECT data FROM events WHERE session_id=? AND seq=?`, r.SessionId, r.Seq).Scan(&b); err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "event is no longer retained")
		}
		return nil, err
	}
	e, err := decode(b)
	if err != nil {
		return nil, err
	}
	out := &api.EventBatch{Events: []*api.Event{e}}
	if !strings.HasPrefix(e.Kind, "tool_") || e.RequestId == "" {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT e.data FROM transcript_records r JOIN events e ON e.session_id=r.session_id AND e.seq=r.seq WHERE r.session_id=? AND r.run_id=? AND (r.request_id=? OR r.related_id=? OR r.request_id IN (SELECT request_id FROM transcript_records WHERE session_id=? AND run_id=? AND related_id=? AND kind='approval')) ORDER BY r.seq`, r.SessionId, e.RunId, e.RequestId, e.RequestId, r.SessionId, e.RunId, e.RequestId)
	if err != nil {
		return nil, err
	}
	out.Events = nil
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			rows.Close()
			return nil, err
		}
		v, x := decode(b)
		if x != nil {
			rows.Close()
			return nil, x
		}
		out.Events = append(out.Events, v)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}

func metadata(e *api.Event) *api.Event {
	if e.Kind != "turn_end" {
		return e
	}
	p := object(e.Payload)
	if len(child(p, "modelUsage")) == 0 {
		return nil
	}
	v := proto.Clone(e).(*api.Event)
	v.Text = ""
	v.Payload, _ = json.Marshal(map[string]any{"modelUsage": p["modelUsage"]})
	return v
}

func pageLimit(limit uint32) int {
	if limit == 0 {
		return DefaultPage
	}
	return int(min(limit, uint32(MaxPage)))
}
