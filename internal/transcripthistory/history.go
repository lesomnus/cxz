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
	"sync/atomic"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/shellview"
	"github.com/lesomnus/cxz/internal/toolview"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const DefaultPage = 256
const MaxPage = 1024

// Heartbeat is how often a row stream with nothing to send says so, by sending
// a row with no event. A reader cannot tell an open stream that is idle from
// one that has stalled -- both are silence -- and only the sending side knows
// which it is; see cxz#188. It lives beside the projection that every row
// stream serves, so a runtime and the sandbox cannot drift on what silence
// means. A reader arms its own deadline at some multiple of this.
const Heartbeat = 10 * time.Second

// metadataKinds are the records a page carries beside its rows. They are not
// rows -- nobody reads a usage record -- but a reader derives from them: what
// quota is left, which models the provider offered, where the context was
// compacted, how much of the window a turn used. Every one of those is a scan
// backwards for the latest record of its kind, so a page that omits a kind
// leaves the thing derived from it blank until the agent speaks again.
//
// `usage_status` is here because quota's state -- polling, unsupported, error
// -- is only in those records, and a client that has usage without them cannot
// tell "no limits reported" from "limits not asked for yet".
//
// `models_status` is deliberately absent: the supervisor writes it, and no
// reader derives anything from the journal copy. The Models RPC answers with
// its own status field.
var metadataKinds = []string{"models", "usage", "usage_status", "compact", "turn_end"}

// metadataPerKind is kept per kind rather than across them. A single budget
// spent in arrival order is spent by whatever is chattiest -- usage and
// turn_end, every turn -- which evicts the one `models` record a long session
// ever wrote, and the compact boundary with it.
const metadataPerKind = 32

type Decode func([]byte) (*api.Event, error)
type Version struct {
	Seq, Updated uint64
	Event        *api.Event
}
type Projection struct {
	Versions []Version
	// resolve finds the row a record belongs to when the reduction does not
	// carry it. Set for the length of one sync, because it reads the table.
	resolve   func(run, request string) *api.Event
	Rows      map[uint64]*api.Event
	tools     map[string]*api.Event
	approvals map[string]string
	questions map[string]*api.Event
	last      map[string]*api.Event
	input     map[string]int64
	inputSeq  map[string]uint64
}

func New() *Projection {
	return &Projection{Rows: map[uint64]*api.Event{}, tools: map[string]*api.Event{}, approvals: map[string]string{}, questions: map[string]*api.Event{}, last: map[string]*api.Event{}, input: map[string]int64{}, inputSeq: map[string]uint64{}}
}

// Cache keeps the reduced state between syncs, so that a change costs the
// records that changed rather than the conversation in front of them. Without
// one, every sync rebuilds that state by reading every current row of the
// session: 2,745 rows and 16.4MB of JSON on a long conversation, 150ms apart
// for each open Rows stream. See docs/plans/incremental-projection.md.
//
// A caller that already serializes a session owns one -- the per-session lock
// is the cache's lock, which is why there is no mutex here. Everyone else
// passes nil and gets the rebuild.
type Cache struct {
	projection  *Projection
	first, last uint64
	format      int
}

// use hands over the cached projection when it was built from exactly the state
// the table is in now. Anything else -- a different format, a trim, a rebuild by
// another process -- and the caller restores instead.
func (c *Cache) use(first, last uint64, format int) *Projection {
	if c == nil || c.projection == nil || c.format != format || c.first != first || c.last != last {
		return nil
	}
	p := c.projection
	p.Versions = p.Versions[:0]
	return p
}
func (c *Cache) keep(p *Projection, first, last uint64, format int) {
	if c == nil {
		return
	}
	c.projection, c.first, c.last, c.format = p, first, last, format
}

// Drop forgets the reduced state. A sync that fails has already applied records
// to it, so what it holds is no longer what the table holds.
func (c *Cache) Drop() {
	if c == nil {
		return
	}
	c.projection = nil
}

func isQuestion(e *api.Event) bool {
	return e.Kind == "approval" && slices.Contains([]string{"AskUserQuestion", "item/tool/requestUserInput", "agentMessage/questions"}, e.Text)
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
	if isQuestion(e) {
		return ""
	}
	if id := text(p, "tool_use_id"); id != "" {
		return id
	}
	return text(child(p, "params"), "itemId")
}

var hidden = map[string]bool{"raw": true, "state": true, "vendor": true, "intent": true, "receipt": true, "approval_resolved": true, "models": true, "models_status": true, "usage": true, "usage_status": true, "history_checkpoint": true, "fork_boundary": true}

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
	// A reader lays a call out by its shape, and the shape is not recoverable
	// from the rest of the row: a Read puts the path it read in Command as well
	// as in Files, so Command being set does not mean a shell ran. Kind says it
	// instead. Description carries the provider's own words when it supplied
	// any -- Bash's description, a Skill's arguments -- and stays empty rather
	// than repeating Name, which a reader already has.
	label.Kind = "tool"
	if activity, ok := toolview.ToolView(provider, e.Text, e.Payload); ok {
		label.Kind, label.Description = activity.Kind, clip(activity.Description, 1024)
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
			if v == nil && e.Kind != "tool_call" && p.resolve != nil {
				// A background task answers after its turn closed, so its call
				// is behind what a reduction carries. One indexed look is
				// cheaper than carrying every call in case of this one.
				v = p.resolve(e.RunId, e.RequestId)
			}
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
		// Every approval row keeps its own request, so a resolution has
		// somewhere to be folded. A question always needed that; so does an
		// approval whose tool never produced a row -- its decision used to be
		// dropped, leaving the row reading "requested" after it was answered.
		p.questions[key] = e
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
		if request := p.questions[key]; request != nil && p.Rows[request.Seq] != nil {
			v := proto.Clone(request).(*api.Event)
			data := object(request.Payload)
			resolution := object(e.Payload)
			resolution["state"] = e.Text
			data["cxz_resolution"] = resolution
			v.Payload, _ = json.Marshal(data)
			p.questions[key] = v
			p.put(v, e.Seq)
		}
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
	// A payload is dropped because it is a native body, loaded on demand by a
	// reader that wants it. Two kinds are exceptions, because their payload is
	// not a body but the whole of what the row says: a question's options, and
	// the floor a trim leaves behind -- without it a client cannot tell that
	// display history below it is gone while the agent's context is not.
	// The kind is spelled out rather than taken from internal/core: that package
	// locks files, which the WASM sandbox cannot build. See core.HistoryTrimmedKind.
	if e.Kind != "approval" && e.Kind != "history_trimmed" {
		v.Payload = nil
	}
	if slices.Contains([]string{"diagnostic", "stderr", "error"}, e.Kind) || e.Kind == "turn_end" && e.Text == "failed" {
		// Error cards need a readable reason even before opening native details.
		// Keep only display metadata, never the potentially large native body.
		data := object(e.Payload)
		native := child(data, "error")
		diagnostic := map[string]any{}
		for _, message := range []string{text(native, "message"), text(data, "message"), text(data, "error")} {
			if strings.TrimSpace(message) != "" {
				diagnostic["message"] = message
				break
			}
		}
		if code := text(native, "codexErrorInfo"); code != "" {
			diagnostic["error"] = map[string]string{"codexErrorInfo": code}
		}
		if retrying, ok := data["willRetry"].(bool); ok {
			diagnostic["willRetry"] = retrying
		}
		if len(diagnostic) > 0 {
			v.Payload, _ = json.Marshal(diagnostic)
		}
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
	// What the record table looked like the last time this session's projection
	// was verified, so that a look which changes nothing can cost nothing.
	`CREATE TABLE IF NOT EXISTS transcript_seen(session_id TEXT PRIMARY KEY,written INTEGER)`,
}

const projectionFormat = 4

// probed is how many times Sync has had to ask whether a record landed behind
// the cursor, restored
// how many times it has had to rebuild the reduced state from the table, and
// restoredRows how many rows that cost. Tests watch them; nothing else does.
var probed, restored, restoredRows, resolved atomic.Uint64

// Sync indexes only native non-raw records. A cache backfill or retention change
// rebuilds this disposable session index; normal queries read only the new suffix.
// openTurn is where the records a reduction can still need begin: the earliest
// input of a run that has not ended. Everything before it belongs to a turn
// that closed, and the three things that read a row by sequence -- an open
// question, and a turn_end folding its metrics onto the response it closes --
// never reach outside their own turn.
//
// Both queries read the kind index and no row bodies, so this costs the turns
// of the conversation rather than its rows. A session with no input at all has
// no turn to be inside, and restores everything, which is nothing much.
func openTurn(ctx context.Context, tx *sql.Tx, id string) (uint64, error) {
	seqByRun := func(kind string) (map[string]uint64, uint64, error) {
		out, top := map[string]uint64{}, uint64(0)
		rows, err := tx.QueryContext(ctx, `SELECT run_id,MAX(seq) FROM transcript_records WHERE session_id=? AND kind=? GROUP BY run_id`, id, kind)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var run string
			var seq uint64
			if err = rows.Scan(&run, &seq); err != nil {
				return nil, 0, err
			}
			out[run] = seq
			if seq > top {
				top = seq
			}
		}
		return out, top, rows.Err()
	}
	inputs, _, err := seqByRun("input")
	if err != nil {
		return 0, err
	}
	if len(inputs) == 0 {
		return 0, nil
	}
	ends, lastEnd, err := seqByRun("turn_end")
	if err != nil {
		return 0, err
	}
	floor := uint64(0)
	for run, seq := range inputs {
		if seq > ends[run] && (floor == 0 || seq < floor) {
			floor = seq
		}
	}
	if floor == 0 {
		// Every run has ended. Nothing before the last close can be needed.
		floor = lastEnd
	}
	return floor, nil
}

func Sync(ctx context.Context, db *sql.DB, id string, decode Decode) error {
	return SyncWith(ctx, db, id, decode, nil)
}

// SyncWith is Sync with somewhere to leave the reduced state. The cache must be
// owned by whatever serializes this session; passing one that is shared without
// a lock is a data race on the projection it holds.
func SyncWith(ctx context.Context, db *sql.DB, id string, decode Decode, cache *Cache) error {
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
	// Rows polls this every 150ms for each open stream, and what it asked in
	// order to notice that nothing had happened was a walk of the whole
	// conversation: counting a 134k-record session took 25ms, so a long
	// conversation spent a third of a core on no news.
	//
	// Ask what is cheap instead. SQLite answers a lone MIN or MAX out of the
	// index, and MAX(rowid) out of the table itself, and together those see
	// everything that happens to a record set which is appended to, trimmed
	// from the front, and backfilled in the middle: a filled hole is a new
	// rowid even though both endpoints stand still. The walk still decides
	// whether a change was an append -- it is simply paid once per change
	// rather than once per look. See cxz#213.
	var written uint64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(rowid),0) FROM events`).Scan(&written); err != nil {
		return err
	}
	var first, last uint64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(seq),0) FROM events WHERE session_id=?`, id).Scan(&first); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events WHERE session_id=?`, id).Scan(&last); err != nil {
		return err
	}
	var oldFirst, oldLast uint64
	err = tx.QueryRowContext(ctx, `SELECT first,last FROM transcript_state WHERE session_id=?`, id).Scan(&oldFirst, &oldLast)
	projected := err == nil
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var seen uint64
	err = tx.QueryRowContext(ctx, `SELECT written FROM transcript_seen WHERE session_id=?`, id).Scan(&seen)
	watermarked := err == nil
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var format int
	err = tx.QueryRowContext(ctx, `SELECT version FROM transcript_format WHERE session_id=?`, id).Scan(&format)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	current := projected && format == projectionFormat
	if current && seen == written && oldFirst == first && oldLast == last {
		return tx.Commit()
	}
	// What remains to decide is whether this is an append, and the question the
	// count used to answer is "did anything land at or behind the cursor".
	// Asking in insertion order reads only the records that are new, where
	// counting the session read all of them. A delete writes no rowid, which is
	// why the endpoints are asked too: a trim moves the first, a purge moves
	// both, and there is no other deletion -- records go in behind the cursor
	// only when the cache is backfilled.
	rebuild := !current || oldFirst != first || last < oldLast
	// A projection made before there was a watermark has nothing to compare
	// against, and no reason to be doubted: the build that made it answered the
	// same question by counting. Adopt the table's mark and carry on, rather
	// than rebuilding every session once on the way past this change.
	if !rebuild && watermarked {
		probed.Add(1)
		behind := false
		// NOT INDEXED, because the rowid range is the index this wants and
		// SQLite otherwise prefers the (session_id,seq) one and scans it --
		// 9.8ms against 0.005ms on a 134k-record session, which is the cost
		// this question exists to avoid.
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events NOT INDEXED WHERE rowid>? AND session_id=? AND seq<=?)`, seen, id, oldLast).Scan(&behind); err != nil {
			return err
		}
		if !behind && oldFirst == first && oldLast == last {
			// The table moved, this session did not. Remember what it looked
			// like, so the next look is the cheap one again.
			if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_seen VALUES(?,?)`, id, written); err != nil {
				return err
			}
			return tx.Commit()
		}
		rebuild = behind
	} else if !rebuild && oldFirst == first && oldLast == last {
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_seen VALUES(?,?)`, id, written); err != nil {
			return err
		}
		return tx.Commit()
	}
	if rebuild {
		oldLast = 0
		cache.Drop()
		for _, table := range []string{"transcript_rows", "transcript_records"} {
			if _, err = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE session_id=?`, id); err != nil {
				return err
			}
		}
	}
	// From here records are applied to the cached projection, so if this sync
	// does not commit, what the cache holds is ahead of what the table does.
	committed := false
	defer func() {
		if !committed {
			cache.Drop()
		}
	}()
	projection := cache.use(oldFirst, oldLast, format)
	warm := projection != nil
	if !warm {
		projection = New()
	}
	if !rebuild && !warm {
		restored.Add(1)
		// Restoring the reduced tools/last responses avoids walking old native
		// bodies, and restoring only the open turn avoids walking the whole
		// conversation to find state that cannot be older than the turn it
		// belongs to. A partner left behind -- a background result whose call
		// closed with an earlier turn -- is fetched when it is missed.
		floor, e := openTurn(ctx, tx, id)
		if e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, `SELECT data FROM transcript_rows r WHERE session_id=? AND seq>=? AND updated=(SELECT MAX(updated) FROM transcript_rows x WHERE x.session_id=r.session_id AND x.seq=r.seq) ORDER BY seq`, id, floor)
		if e != nil {
			return e
		}
		for rows.Next() {
			restoredRows.Add(1)
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
			if isQuestion(&v) {
				projection.questions[identity(v.RunId, v.RequestId)] = &v
			}
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
	// For the length of the apply, a missed partner can be looked up. The
	// closure reads this transaction, so it does not outlive it -- and must not
	// be left on a projection that is about to be cached.
	projection.resolve = func(run, request string) *api.Event {
		var b []byte
		e := tx.QueryRowContext(ctx, `SELECT data FROM transcript_rows WHERE session_id=? AND seq=(SELECT MIN(seq) FROM transcript_records WHERE session_id=? AND run_id=? AND request_id=?) ORDER BY updated DESC LIMIT 1`, id, id, run, request).Scan(&b)
		if e != nil {
			return nil
		}
		var v api.Event
		if json.Unmarshal(b, &v) != nil || v.ToolSummary == nil {
			return nil
		}
		resolved.Add(1)
		return &v
	}
	for _, e := range native {
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_records VALUES(?,?,?,?,?,?)`, id, e.Seq, e.RunId, e.RequestId, e.Kind, related(e)); err != nil {
			return err
		}
		projection.Apply(e)
	}
	projection.resolve = nil
	for _, v := range projection.Versions {
		b, e := json.Marshal(v.Event)
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_rows VALUES(?,?,?,?,?)`, id, v.Seq, v.Updated, v.Event.Kind, b); err != nil {
			return err
		}
	}
	// Named columns, and no count: the column stays so that a rollback to a
	// build which reads it sees a zero and rebuilds once, rather than a NULL it
	// cannot scan.
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_state(session_id,count,first,last) VALUES(?,0,?,?)`, id, first, last); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_seen VALUES(?,?)`, id, written); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO transcript_format VALUES(?,?)`, id, projectionFormat); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	cache.keep(projection, first, last, projectionFormat)
	committed = true
	return nil
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
	// Ranked within each kind, so the newest of a rare kind survives a session
	// that wrote thousands of a common one. Ordered ascending on the way out,
	// which is how a reader merges them.
	meta := []any{r.SessionId, out.SnapshotSeq}
	for _, kind := range metadataKinds {
		meta = append(meta, kind)
	}
	meta = append(meta, metadataPerKind)
	rows, err = db.QueryContext(ctx, `SELECT data FROM (SELECT e.data AS data, r.seq AS seq, ROW_NUMBER() OVER (PARTITION BY r.kind ORDER BY r.seq DESC) AS rank`+
		` FROM transcript_records r JOIN events e ON e.session_id=r.session_id AND e.seq=r.seq`+
		` WHERE r.session_id=? AND r.seq<=? AND r.kind IN (`+placeholders(len(metadataKinds))+`)) WHERE rank<=? ORDER BY seq`, meta...)
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
	return out, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// keepPerKind is what the SQL above does with a window function, for the
// in-memory path that has no SQL. Order is preserved, so the result is still
// ascending by seq.
func keepPerKind(all []*api.Event) []*api.Event {
	seen := map[string]int{}
	for _, e := range all {
		seen[e.Kind]++
	}
	drop := map[string]int{}
	for kind, n := range seen {
		if n > metadataPerKind {
			drop[kind] = n - metadataPerKind
		}
	}
	if len(drop) == 0 {
		return all
	}
	out := make([]*api.Event, 0, len(all))
	for _, e := range all {
		if drop[e.Kind] > 0 {
			drop[e.Kind]--
			continue
		}
		out = append(out, e)
	}
	return out
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

// Tail is the projection as it arrives, for a client that is reading rather
// than deriving. It answers with every row version published after a cursor,
// in publication order.
//
// A cursor over `updated` rather than over `seq` is what makes a stream of rows
// possible at all: folding a turn's metrics onto its response publishes that
// response again, at the sequence of the record that completed it. Ordering by
// updated therefore delivers the correction after the row it corrects, and a
// reader replaces by seq. Page asks the same table for one version per seq as
// of a snapshot; this asks it for the versions themselves.
//
// The data column holds the row as Page returns it, so a row from here and the
// same row from a page are the same bytes. That is the property the clients
// depend on, and history_test.go pins it.
func Tail(ctx context.Context, db *sql.DB, id string, afterUpdated uint64, limit int, decode Decode) ([]*api.Row, error) {
	if limit <= 0 {
		limit = 256
	}
	rows, err := db.QueryContext(ctx, `SELECT seq,updated,data FROM transcript_rows WHERE session_id=? AND updated>? ORDER BY updated ASC,seq ASC LIMIT ?`, id, afterUpdated, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*api.Row
	for rows.Next() {
		var seq, updated uint64
		var b []byte
		if err = rows.Scan(&seq, &updated, &b); err != nil {
			return nil, err
		}
		var e api.Event
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		out = append(out, &api.Row{Event: &e, Updated: updated})
	}
	return out, rows.Err()
}
