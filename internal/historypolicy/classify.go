package historypolicy

import (
	"encoding/json"
	"strings"
)

// Record is one recorded thing, as the classifier needs to see it: the wire
// kind that carried it, the subtype, and -- where a kind mixes meanings in one
// payload -- the field the content sits in.
//
// Field is a path like `item.arguments`, or the leaf alone where a provider
// nests the same content differently; both forms are listed where they differ.
// A caller classifying a whole event leaves it empty and gets the category the
// event belongs to; a caller splitting a payload asks per field. Splitting is
// what keeps a tool call's name in a category that outlives its megabyte of
// arguments.
type Record struct {
	Kind    string
	Text    string
	Field   string
	RawBody []byte // the verbatim payload, for raw only
}

// Classify answers the category a record belongs to.
//
// An unknown kind is deliberately not an error and not dropped: it is
// diagnostic history. A provider that adds an event cxz has never seen should
// cost a bounded amount and stay readable, because a new event quietly deleted
// is worse than a new event quietly counted.
func Classify(r Record) Category {
	switch r.Kind {
	case "input", "assistant", "message", "compact":
		return CategoryConversation
	case "tool_output":
		// Entirely text in the measurement, and it is what a person reads.
		return CategoryConversation
	case "turn_end":
		return CategoryTurnMetrics
	case "usage":
		return classifyUsage(r)
	case "models":
		return CategoryModelCatalog
	case "usage_status", "models_status", "setting_status":
		return CategoryAvailability
	case "tool_call":
		return classifyToolCall(r)
	case "tool_result":
		return classifyToolResult(r)
	case "approval":
		return classifyApproval(r)
	case "raw":
		return classifyRaw(r)
	case "state", "intent", "receipt", "permission", "setting", "queued",
		"background", "approval_resolved", "history_checkpoint", "fork_boundary":
		return CategoryControl
	case "diagnostic", "error", "update", "history_shed", "history_trimmed", "vendor":
		return CategoryDiagnostic
	}
	return CategoryDiagnostic
}

// An account's remaining quota and a turn's token spend arrive as the same
// kind. One is a current value whose past is noise; the other is what a turn
// cost and is never overwritten.
func classifyUsage(r Record) Category {
	switch r.Text {
	case "get_usage", "account/rateLimits/updated", "rate_limit_event":
		return CategoryAccountLimits
	case "thread/tokenUsage/updated", "context/message":
		return CategoryTurnMetrics
	}
	return CategoryAccountLimits
}

func classifyToolCall(r Record) Category {
	switch r.Field {
	case "", "item", "item.type", "item.id", "item.cwd", "threadId", "turnId",
		"tool_use_id", "name", "description", "file_path", "item.command":
		return CategoryWorkSummary
	case "item.changes", "changes":
		return CategoryWorkDiff
	}
	// Arguments, content, patches: whatever else a call carries is its detail.
	return CategoryWorkDetail
}

func classifyToolResult(r Record) Category {
	switch r.Field {
	case "", "item", "item.type", "item.id", "tool_use_id", "status", "is_error":
		return CategoryWorkSummary
	case "item.changes", "changes":
		return CategoryWorkDiff
	}
	return CategoryWorkDetail
}

// An approval is three things at once: a decision worth keeping, a pending
// state a restart needs, and a list of options the provider repeats verbatim on
// every call.
func classifyApproval(r Record) Category {
	switch r.Field {
	case "params.availableDecisions", "availableDecisions",
		"params.proposedExecpolicyAmendment", "proposedExecpolicyAmendment",
		"permission_suggestions":
		return CategoryProviderConstant
	case "", "params.itemId", "params.threadId", "params.turnId", "method",
		"tool_use_id", "subtype", "decision_reason", "decision_reason_type",
		"matched_ask_rule", "description":
		return CategoryWorkSummary
	}
	return CategoryWorkDetail
}

// Raw is not one thing. Most of what survives retention is the provider's
// replies to cxz's own polling -- the measurement found `control_response` at
// 79.5% of a claude session's raw, and id/result shapes at 61.7-99.6% of each
// codex session's -- which is a second copy of what the normalized events
// already carry. The rest is the stream that produced the conversation.
func classifyRaw(r Record) Category {
	if len(r.RawBody) == 0 {
		// A shed body says nothing about which it was. The reader is told
		// `expired`; the classifier does not guess a category from emptiness.
		return CategoryProviderStream
	}
	var v struct {
		Type   string          `json:"type"`
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(r.RawBody, &v) != nil {
		return CategoryProviderStream
	}
	if v.Type == "control_response" || v.Type == "control_request" {
		return CategoryProviderReply
	}
	// A JSON-RPC reply: an id with a result or an error, and no method of its
	// own. A notification has a method and is part of the stream.
	if v.Method == "" && len(v.ID) > 0 && (len(v.Result) > 0 || len(v.Error) > 0) {
		return CategoryProviderReply
	}
	return CategoryProviderStream
}

// LatestKey is the semantic identity of a `keep: latest` record -- what the
// present value is the present value *of*.
//
// Separate windows stay separate: a weekly limit and a five-hour limit are two
// values, and folding them into one would make the display answer for the
// wrong one. A caller passes the parts it has; empty parts are dropped.
func LatestKey(c Category, parts ...string) string {
	kept := make([]string, 0, len(parts)+1)
	kept = append(kept, string(c))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "/")
}
