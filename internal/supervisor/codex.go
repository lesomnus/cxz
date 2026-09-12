package supervisor

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"strings"
)

type codexProtocol struct {
	s     *Supervisor
	turn  string
	token func(previous string, refresh bool) (accounts.Token, error)
}

func rpc(id, method string, params any) any {
	return map[string]any{"id": id, "method": method, "params": params}
}
func (c *codexProtocol) consume(raw []byte) {
	s := c.s
	var v struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &v) != nil {
		s.event("diagnostic", "invalid Codex JSON", "", nil, nil)
		return
	}
	id := string(v.ID)
	// Account telemetry is best-effort and must never fail an active turn.
	if id == `"cxz-quota"` {
		if len(v.Result) > 0 && string(v.Result) != "null" {
			s.event("usage", "account/rateLimits/updated", "", v.Result, nil)
		}
		var failure struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(v.Error, &failure) == nil && failure.Code == -32601 {
			s.quotaDisabled = true
			s.event("usage_status", "unsupported", "", nil, nil)
		} else if len(v.Error) > 0 && string(v.Error) != "null" {
			s.event("usage_status", "error", "", nil, nil)
		}
		return
	}
	if len(v.Error) > 0 && string(v.Error) != "null" {
		if id == `"cxz-auth"` {
			s.event("diagnostic", "central Codex login failed", "", nil, nil)
			s.event("state", "failed", "", nil, nil)
			s.kill()
			return
		}
		s.event("diagnostic", "Codex request failed", "", v.Error, nil)
		if s.snap.State == "starting" {
			s.event("state", "failed", "", nil, nil)
			s.kill()
		} else {
			s.clearPending()
			s.event("turn_end", "failed", "", v.Error, nil)
			s.event("state", "idle", "", nil, nil)
		}
		return
	}
	switch id {
	case `"cxz-initialize"`:
		_ = s.write(map[string]any{"method": "initialized"})
		if c.token != nil {
			token, err := c.token("", false)
			if err != nil {
				s.event("diagnostic", "central authentication unavailable; check account login", "", nil, nil)
				s.event("state", "failed", "", nil, nil)
				s.kill()
				return
			}
			_ = s.write(rpc("cxz-auth", "account/login/start", map[string]any{"type": "chatgptAuthTokens", "accessToken": token.AccessToken, "chatgptAccountId": token.AccountID, "chatgptPlanType": token.PlanType}))
			return
		}
		c.startThread()
		return
	case `"cxz-auth"`:
		c.startThread()
		return
	case `"cxz-thread"`:
		var r struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if json.Unmarshal(v.Result, &r) != nil || r.Thread.ID == "" {
			s.event("state", "failed", "", nil, nil)
			s.kill()
			return
		}
		s.snap.VendorID = r.Thread.ID
		s.event("vendor", r.Thread.ID, "", nil, nil)
		s.event("state", "idle", "", nil, nil)
		c.readQuota()
		return
	}
	if len(v.ID) > 0 && v.Method != "" {
		switch v.Method {
		case "account/chatgptAuthTokens/refresh":
			var p struct {
				PreviousAccountID string `json:"previousAccountId"`
			}
			var token accounts.Token
			err := fmt.Errorf("external authentication not configured")
			if c.token != nil && json.Unmarshal(v.Params, &p) == nil {
				token, err = c.token(p.PreviousAccountID, true)
			}
			if err != nil {
				_ = s.write(map[string]any{"id": v.ID, "error": map[string]any{"code": -32000, "message": "central authentication unavailable; check account login"}})
				s.event("diagnostic", "central authentication refresh failed", "", nil, nil)
			} else {
				_ = s.write(map[string]any{"id": v.ID, "result": token})
			}
		case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval", "item/tool/requestUserInput":
			p := s.event("approval", v.Method, id, map[string]any{"method": v.Method, "id": v.ID, "params": v.Params}, nil)
			s.pending[id] = p
			s.event("state", "waiting_input", "", nil, nil)
		default: // Unknown server calls are never auto-approved.
			_ = s.write(map[string]any{"id": v.ID, "error": map[string]any{"code": -32601, "message": "cxz does not support this server request; use project-local login for authentication"}})
			s.event("diagnostic", "unsupported Codex request: "+v.Method, "", nil, nil)
		}
		return
	}
	var p struct {
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
		Item struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Text    string `json:"text"`
			Command string `json:"command"`
			Output  string `json:"aggregatedOutput"`
		} `json:"item"`
	}
	_ = json.Unmarshal(v.Params, &p)
	switch v.Method {
	case "turn/started":
		c.turn = p.Turn.ID
		s.event("state", "working", "", nil, nil)
	case "turn/completed":
		s.clearPending()
		state := "completed"
		if p.Turn.Status == "failed" {
			state = "failed"
		}
		if p.Turn.Status == "interrupted" || s.interrupted {
			state = "interrupted"
		}
		s.interrupted = false
		c.turn = ""
		s.event("turn_end", state, "", v.Params, nil)
		s.event("state", "idle", "", nil, nil)
		c.readQuota()
	case "item/started":
		if p.Item.Type == "commandExecution" || p.Item.Type == "fileChange" {
			s.event("tool_call", p.Item.Command, p.Item.ID, v.Params, nil)
		}
	case "item/completed":
		if p.Item.Type == "contextCompaction" {
			s.event("compact", "completed", p.Item.ID, v.Params, nil)
		} else if p.Item.Type == "agentMessage" {
			s.event("assistant", p.Item.Text, p.Item.ID, v.Params, nil)
		} else if p.Item.Type == "commandExecution" || p.Item.Type == "fileChange" {
			s.event("tool_result", p.Item.Output, p.Item.ID, v.Params, nil)
		}
	case "thread/tokenUsage/updated", "account/rateLimits/updated":
		s.event("usage", v.Method, "", v.Params, nil)
	case "error":
		s.event("diagnostic", "Codex error", "", v.Params, nil)
	}
}
func (c *codexProtocol) startThread() {
	s := c.s
	params := map[string]any{"cwd": s.session.Workspace, "approvalPolicy": "untrusted", "sandbox": "danger-full-access", "experimentalRawEvents": false, "persistExtendedHistory": true}
	method := "thread/start"
	if s.session.Model != "" {
		params["model"] = s.session.Model
	}
	if s.snap.VendorID != "" {
		method = "thread/resume"
		params["threadId"] = s.snap.VendorID
	}
	_ = s.write(rpc("cxz-thread", method, params))
}

func (c *codexProtocol) readQuota() {
	if !c.s.quotaDisabled {
		_ = c.s.write(rpc("cxz-quota", "account/rateLimits/read", map[string]any{}))
	}
}
func (c *codexProtocol) command(op string, v core.Command) (any, error) {
	s := c.s
	switch op {
	case "send":
		if s.snap.State != "idle" || strings.TrimSpace(v.Text) == "" {
			return nil, fmt.Errorf("session must be idle and text nonempty")
		}
		if strings.TrimSpace(v.Text) == "/compact" {
			return rpc(v.ClientID, "thread/compact/start", map[string]any{"threadId": s.snap.VendorID}), nil
		}
		return rpc(v.ClientID, "turn/start", map[string]any{"threadId": s.snap.VendorID, "input": []any{map[string]any{"type": "text", "text": v.Text, "text_elements": []any{}}}}), nil
	case "interrupt":
		if c.turn == "" {
			return nil, fmt.Errorf("no active Codex turn")
		}
		return rpc(v.ClientID, "turn/interrupt", map[string]any{"threadId": s.snap.VendorID, "turnId": c.turn}), nil
	case "stop":
		return nil, nil
	case "reply":
		p, ok := s.pending[v.RequestID]
		if !ok {
			return nil, fmt.Errorf("approval is stale or already resolved")
		}
		var r struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Questions []struct {
					ID       string `json:"id"`
					Question string `json:"question"`
				} `json:"questions"`
				Permissions json.RawMessage `json:"permissions"`
			} `json:"params"`
		}
		if e := json.Unmarshal(p.Payload, &r); e != nil {
			return nil, e
		}
		result := map[string]any{}
		switch r.Method {
		case "item/tool/requestUserInput":
			answers := map[string]any{}
			for _, q := range r.Params.Questions {
				answer := v.Answers[q.ID]
				if answer == "" {
					answer = v.Answers[q.Question]
				}
				if v.Allow && answer == "" {
					return nil, fmt.Errorf("answer required for %q (%s)", q.Question, q.ID)
				}
				a := []string{}
				if v.Allow {
					a = append(a, answer)
				}
				answers[q.ID] = map[string]any{"answers": a}
			}
			result["answers"] = answers
		case "item/permissions/requestApproval":
			permissions := json.RawMessage(`{}`)
			if v.Allow {
				permissions = r.Params.Permissions
			}
			result["permissions"] = permissions
			result["scope"] = "turn"
		default:
			decision := "decline"
			if v.Allow {
				decision = "accept"
			}
			result["decision"] = decision
		}
		return map[string]any{"id": r.ID, "result": result}, nil
	}
	return nil, fmt.Errorf("unknown command")
}
