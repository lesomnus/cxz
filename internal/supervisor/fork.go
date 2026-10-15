package supervisor

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/lesomnus/cxz/internal/sessionfork"
)

// Serialize the native snapshot with sends and settings. The parent can keep
// running after this returns; the child never reads its mutable profile again.
func (s *Supervisor) serveFork(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/fork" {
		return false
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return true
	}
	var request sessionfork.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		http.Error(w, "invalid fork request", 400)
		return true
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(45 * time.Second))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settingPending != "" || len(s.pending) != 0 {
		http.Error(w, "resolve pending settings or questions before forking", 409)
		return true
	}
	child, err := sessionfork.Prepare(r.Context(), s.quotaRoot, s.session, s.snap, s.log.All(), request)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return true
	}
	json.NewEncoder(w).Encode(child)
	return true
}
