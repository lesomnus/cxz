package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type Session struct {
	Supervisor      string      `json:"supervisor,omitempty"`
	CreateID        string      `json:"create_id"`
	ID              string      `json:"id"`
	Workspace       string      `json:"workspace"`
	Title           string      `json:"title"`
	CreatedAt       int64       `json:"created_at"`
	Agent           string      `json:"agent"`
	Kind            string      `json:"kind,omitempty"`
	Model           string      `json:"model,omitempty"`
	ProjectID       string      `json:"project_id,omitempty"`
	LegacyProjectID string      `json:"legacy_project_id,omitempty"`
	Account         string      `json:"account,omitempty"`
	AuthBackend     string      `json:"auth_backend"`
	AuthBinding     string      `json:"auth_binding"`
	Fork            *ForkOrigin `json:"fork,omitempty"`
}

// ForkOrigin is private runtime metadata. NativePath refers to the child's own
// snapshot of the provider transcript, never a mutable file in the parent.
type ForkOrigin struct {
	SessionID  string `json:"session_id"`
	RunID      string `json:"run_id"`
	VendorID   string `json:"vendor_id,omitempty"`
	ThroughSeq uint64 `json:"through_seq"`
	NativePath string `json:"native_path,omitempty"`
}

const ForkBoundaryKind = "fork_boundary"

type Event struct {
	SessionID string            `json:"session_id"`
	RunID     string            `json:"run_id"`
	Seq       uint64            `json:"seq"`
	TimeMS    int64             `json:"time_ms"`
	Kind      string            `json:"kind"`
	Text      string            `json:"text,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Payload   json.RawMessage   `json:"payload,omitempty"`
	Raw       []byte            `json:"raw,omitempty"` // Base64 preserves the exact vendor bytes.
	Response  *ResponseMetadata `json:"response,omitempty"`
	// Agent is the subagent whose work this is, absent when the session did it
	// itself. A subagent's tool calls and replies arrive on the session's own
	// stream and are recorded there, so without this nothing in a record says
	// which of several concurrent subagents a line came from -- or that it came
	// from one at all. See cxz#368.
	Agent *EventAgent `json:"agent,omitempty"`
}

// EventAgent identifies one subagent run. ID is the id of the tool call that
// started it, which is how an event is tied back to the `Agent` call above it
// and how two subagents running in the same turn stay apart.
type EventAgent struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
}

// ResponseMetadata is immutable context for this response, not the session's
// current settings. Sources distinguish response/settings reports from requests.
type ResponseMetadata struct {
	Model          string `json:"model,omitempty"`
	Effort         string `json:"effort,omitempty"`
	ModelSource    string `json:"model_source,omitempty"`
	EffortSource   string `json:"effort_source,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	Phase          string `json:"phase,omitempty"`
	CompletionJSON string `json:"completion_json,omitempty"`
}

// ResponseCompletion belongs to a successful turn_end and references its final
// assistant event. Unknown metrics remain absent; token subsets are not added twice.
type ResponseCompletion struct {
	ResponseSeq    uint64             `json:"response_seq,string"`
	FinalSource    string             `json:"final_source"`
	DurationMS     *int64             `json:"duration_ms,omitempty"`
	DurationSource string             `json:"duration_source,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	TokenScope     string             `json:"token_scope,omitempty"`
}
type Snapshot struct {
	PermissionMode string  `json:"permission_mode"`
	State          string  `json:"state"`
	RunID          string  `json:"run_id"`
	VendorID       string  `json:"vendor_id"`
	LastSeq        uint64  `json:"last_seq"`
	Pending        []Event `json:"pending"`
	// One message may wait for an agent that cannot take it yet. It is part of
	// the projection rather than a client's memory, so every frontend sees the
	// same one waiting, and a supervisor restart does not lose it.
	Queued   string `json:"queued,omitempty"`
	QueuedID string `json:"queued_id,omitempty"`
}
type Command struct {
	Busy       bool                       `json:"busy,omitempty"`
	RunID      string                     `json:"run_id"`
	ClientID   string                     `json:"client_id"`
	Text       string                     `json:"text,omitempty"`
	RequestID  string                     `json:"request_id,omitempty"`
	Allow      bool                       `json:"allow,omitempty"`
	Answers    map[string]string          `json:"answers,omitempty"`
	Selections map[string]AnswerSelection `json:"selections,omitempty"`
}
type Receipt struct {
	ClientID string `json:"client_id"`
	Status   string `json:"status"`
}

func ID() string {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

// SessionID reserves the same identity in the manager and project database
// before either side has started a process or written a runtime manifest.
func SessionID(clientID string) string {
	h := sha256.Sum256([]byte("cxz.session:" + clientID))
	return hex.EncodeToString(h[:12])
}

func Dir(root, id string) string    { return filepath.Join(root, "sessions", id) }
func Socket(root, id string) string { return filepath.Join(root, "run", id+".sock") }
func Prepare(root string) error {
	for _, p := range []string{root, filepath.Join(root, "sessions"), filepath.Join(root, "run")} {
		if e := os.MkdirAll(p, 0700); e != nil {
			return e
		}
		if e := os.Chmod(p, 0700); e != nil {
			return e
		}
	}
	return nil
}
func WriteJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return WriteFile(path, b)
}

// WriteFile replaces a private file after syncing its contents.
func WriteFile(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".cxz-write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	return SyncDir(filepath.Dir(path))
}
