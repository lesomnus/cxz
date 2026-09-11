package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

type Session struct {
	CreateID  string `json:"create_id"`
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	Agent     string `json:"agent"`
	Kind      string `json:"kind,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	ConfigDir string `json:"config_dir,omitempty"`
}
type Event struct {
	SessionID string          `json:"session_id"`
	RunID     string          `json:"run_id"`
	Seq       uint64          `json:"seq"`
	TimeMS    int64           `json:"time_ms"`
	Kind      string          `json:"kind"`
	Text      string          `json:"text,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Raw       []byte          `json:"raw,omitempty"` // Base64 preserves the exact vendor bytes.
}
type Snapshot struct {
	State    string  `json:"state"`
	RunID    string  `json:"run_id"`
	VendorID string  `json:"vendor_id"`
	LastSeq  uint64  `json:"last_seq"`
	Pending  []Event `json:"pending"`
}
type Command struct {
	RunID     string            `json:"run_id"`
	ClientID  string            `json:"client_id"`
	Text      string            `json:"text,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Allow     bool              `json:"allow,omitempty"`
	Answers   map[string]string `json:"answers,omitempty"`
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

func SyncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func Lock(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("already running: " + path)
	}
	return f, nil
}
