// Package quotashare coordinates account quota polling without sharing credentials.
package quotashare

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

type Request struct {
	Project, Session, Account, Lease string
	Payload                          json.RawMessage
}
type Snapshot struct {
	Payload   json.RawMessage
	Observed  int64
	Requested int64
	Lease     string
}
type Response struct {
	Snapshot Snapshot
	Poll     bool
}

// An expiring claim prevents duplicate requests across processes, including
// after a leader exits. Empty/failed responses do not erase the last good data.
func Exchange(root string, r Request, now time.Time) (Response, error) {
	dir := filepath.Join(root, "quota")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Response{}, err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Account)))
	lock, err := os.OpenFile(filepath.Join(dir, key+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Response{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Response{}, err
	}
	path := filepath.Join(dir, key+".json")
	var state Snapshot
	raw, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(raw, &state); err != nil {
			return Response{}, err
		}
	} else if !os.IsNotExist(err) {
		return Response{}, err
	}
	out := Response{}
	changed := false
	if r.Lease != "" {
		if state.Lease != r.Lease {
			state.Lease = ""
			return Response{Snapshot: state}, nil
		}
		if len(r.Payload) > 0 {
			state.Payload = r.Payload
			state.Observed = now.UnixMilli()
			changed = true
		}
	} else if now.UnixMilli()-state.Requested >= time.Minute.Milliseconds() || state.Requested == 0 {
		state.Requested = now.UnixMilli()
		state.Lease = core.ID()
		out.Poll = true
		changed = true
	}
	if changed {
		if err = core.WriteJSON(path, state); err != nil {
			return Response{}, err
		}
	}
	out.Snapshot = state
	// Only the polling owner gets the publication capability.
	if !out.Poll {
		out.Snapshot.Lease = ""
	}
	return out, nil
}
