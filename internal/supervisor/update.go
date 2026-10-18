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
	"github.com/lesomnus/cxz/internal/cxzupdate"
)

// UpdateIdlePeriod is how long a session must be quiet before an update may
// take it. A var, and overridable, because verifying a rollout end to end
// otherwise costs five idle minutes per attempt -- which is most of why four
// separate blockers took an hour to walk through. See cxz#224.
var UpdateIdlePeriod = cxzupdate.Tunable("CXZ_UPDATE_IDLE", 5*time.Minute)

type updateClient struct {
	seen time.Time
	busy bool
}
type UpdateStatus struct {
	SupervisorBinary, Revision string
	Protocol                   int32
	Ready                      bool
	Reason, Binary, State      string
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
		// A block earns another full idle period, because the agent did
		// something. These two did not: one is the clock itself, and the other
		// is a person looking at the screen. See cxz#224.
		if reason != "" && reason != "waiting for 5 minutes of continuous idle" && reason != "attached client busy" {
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
	if s.updateBackground.Unavailable {
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
			// Refuse this attempt, but leave the clock alone. A TUI reports
			// itself busy for ten seconds after any keypress -- scrolling a
			// transcript counts -- so resetting the idle period here meant a
			// person reading a conversation kept the installation from ever
			// updating. "Someone is mid-interaction" and "nothing has been
			// quiet for five minutes" are different answers, and only the
			// second one needs a timer. See cxz#224.
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

// processQuiet reports whether a process found in the agent's tree is
// something an update may ignore. A process cxz asked the agent to spawn lives
// exactly as long as the session, so counting it means the session is never
// quiet and the update never happens; a process that has already exited is not
// doing work, and nothing guarantees it gets reaped -- PID 1 of a session
// container is `sleep infinity`.
func processQuiet(state string, cmdline []byte, id string) bool {
	if state == "Z" {
		return true
	}
	args := strings.Split(strings.TrimSuffix(string(cmdline), "\x00"), "\x00")
	for i, a := range args {
		if a == "_mcp-bridge" && i+1 < len(args) && args[i+1] == id {
			return true
		}
	}
	return false
}

// procState reports the state and process group of pid. A pid that is gone
// between a listing and the read reports an exited state: it is not work.
func procState(pid int) (state string, group int, err error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if os.IsNotExist(err) {
		return "Z", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	end := strings.LastIndexByte(string(b), ')')
	if end < 0 {
		return "", 0, fmt.Errorf("unreadable stat of %d", pid)
	}
	fields := strings.Fields(string(b[end+1:]))
	if len(fields) < 3 {
		return "", 0, fmt.Errorf("unreadable stat of %d", pid)
	}
	if group, err = strconv.Atoi(fields[2]); err != nil {
		return "", 0, err
	}
	return fields[0], group, nil
}

func (s *Supervisor) procQuiet(pid int, state string) bool {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	return processQuiet(state, cmdline, s.session.ID)
}

func (s *Supervisor) processesQuiet() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	pid := s.cmd.Process.Pid
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", pid, pid))
	if err != nil {
		return false
	}
	for _, f := range strings.Fields(string(b)) {
		child, err := strconv.Atoi(f)
		if err != nil {
			return false
		}
		state, _, err := procState(child)
		if err != nil || !s.procQuiet(child, state) {
			return false
		}
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
		state, group, err := procState(n)
		if err != nil {
			return false
		}
		if group == pid && !s.procQuiet(n, state) {
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
		// A client attaching is an interaction; a client reporting that it is
		// busy is not, and it already refuses the attempt on its own.
		if _, known := s.updateClients[c.ClientID]; !known {
			s.lastActivity = time.Now()
		}
		s.updateClients[c.ClientID] = updateClient{time.Now(), c.Busy}
		json.NewEncoder(w).Encode(core.Receipt{ClientID: c.ClientID, Status: "accepted"})
	case "/update-notice":
		s.event("update", c.Text, "", nil, nil)
		json.NewEncoder(w).Encode(core.Receipt{Status: "accepted"})
	case "/update-status":
		reason := s.updateReason(time.Now(), s.processesQuiet())
		json.NewEncoder(w).Encode(UpdateStatus{Ready: reason == "", Reason: reason, Binary: s.session.Agent, State: s.snap.State, SupervisorBinary: s.session.Supervisor, Revision: cxzupdate.Current().Revision, Protocol: cxzupdate.Protocol})
	}
	return true
}
