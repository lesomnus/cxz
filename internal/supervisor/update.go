package supervisor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
)

const UpdateIdlePeriod = 5 * time.Minute

type updateClient struct {
	seen time.Time
	busy bool
}
type UpdateStatus struct {
	Ready                 bool
	Reason, Binary, State string
}

func (s *Supervisor) observeUpdateEvent(kind, text, id string) {
	switch kind {
	case "state":
		if text != "idle" || s.snap.State != "idle" {
			s.lastActivity = time.Now()
		}
	case "input", "approval", "approval_resolved", "tool", "tool_result", "assistant", "turn_end", "intent":
		s.lastActivity = time.Now()
	}
	if kind == "tool" && id != "" {
		if s.updateTools == nil {
			s.updateTools = map[string]bool{}
		}
		s.updateTools[id] = true
	}
	if kind == "tool_result" && id != "" {
		delete(s.updateTools, id)
	}
}
func (s *Supervisor) observeUpdateRaw(raw []byte) {
	if agentview.IsBackgroundEvent(raw) {
		s.updateBackground.Apply(raw)
		s.lastActivity = time.Now()
	}
	// Unknown background protocols cannot be assumed empty. The native process
	// group is checked independently, including Codex background shell children.
	var e struct{ Method, Type, Subtype string }
	if json.Unmarshal(raw, &e) != nil {
		s.updateUnknown = true
		return
	}
	if strings.Contains(strings.ToLower(e.Method), "background") || (strings.Contains(e.Subtype, "task") && !agentview.IsBackgroundEvent(raw) && e.Type == "system") {
		s.updateUnknown = true
	}
}

func (s *Supervisor) updateReason(now time.Time, quiet bool) (reason string) {
	defer func() {
		if reason != "" && reason != "waiting for 5 minutes of continuous idle" {
			s.updateBlocked = true
		}
	}()
	if s.snap.State != "idle" || s.stopping {
		return "agent not idle"
	}
	if len(s.pending) > 0 {
		return "pending approvals/questions"
	}
	if s.settingPending != "" {
		return "model/effort change pending"
	}
	if s.codex != nil && (s.codex.turn != "" || len(s.codex.asyncReplies) > 0) {
		return "queued Codex turn/answer"
	}
	if s.updateUnknown {
		return "background telemetry unknown"
	}
	if len(s.updateTools) > 0 {
		return "unfinished tool calls"
	}
	for _, task := range s.updateBackground.Tasks {
		if task.Active {
			return "background tasks running"
		}
	}
	if !quiet {
		s.lastActivity = now
		return "agent child processes running or unknown"
	}
	for id, c := range s.updateClients {
		if now.Sub(c.seen) > 30*time.Second {
			delete(s.updateClients, id)
			s.lastActivity = now
			continue
		}
		if c.busy {
			s.lastActivity = now
			return "attached client busy"
		}
	}
	if s.updateBlocked {
		s.updateBlocked = false
		s.lastActivity = now
	}
	if s.lastActivity.IsZero() || now.Sub(s.lastActivity) < UpdateIdlePeriod {
		return "waiting for 5 minutes of continuous idle"
	}
	return ""
}

func (s *Supervisor) processesQuiet() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	pid := s.cmd.Process.Pid
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", pid, pid))
	if err != nil || len(strings.Fields(string(b))) > 0 {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err != nil || n == pid {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false
		}
		end := strings.LastIndexByte(string(b), ')')
		if end < 0 {
			return false
		}
		fields := strings.Fields(string(b[end+1:]))
		if len(fields) < 3 {
			return false
		}
		group, err := strconv.Atoi(fields[2])
		if err != nil {
			return false
		}
		if group == pid {
			return false
		}
	}
	return true
}

func (s *Supervisor) serveUpdate(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/update-status" && r.URL.Path != "/activity" && r.URL.Path != "/update-notice" {
		return false
	}
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return true
	}
	var c core.Command
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&c) != nil {
		http.Error(w, "invalid maintenance request", 400)
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.RunID != s.snap.RunID {
		http.Error(w, "stale run", 409)
		return true
	}
	switch r.URL.Path {
	case "/activity":
		if c.ClientID == "" || len(c.ClientID) > 128 {
			http.Error(w, "client ID required", 400)
			return true
		}
		if s.updateClients == nil {
			s.updateClients = map[string]updateClient{}
		}
		if _, known := s.updateClients[c.ClientID]; !known || c.Busy {
			s.lastActivity = time.Now()
		}
		s.updateClients[c.ClientID] = updateClient{time.Now(), c.Busy}
		json.NewEncoder(w).Encode(core.Receipt{ClientID: c.ClientID, Status: "accepted"})
	case "/update-notice":
		s.event("update", c.Text, "", nil, nil)
		json.NewEncoder(w).Encode(core.Receipt{Status: "accepted"})
	case "/update-status":
		reason := s.updateReason(time.Now(), s.processesQuiet())
		json.NewEncoder(w).Encode(UpdateStatus{Ready: reason == "", Reason: reason, Binary: s.session.Agent, State: s.snap.State})
	}
	return true
}
