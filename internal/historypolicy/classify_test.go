package historypolicy

import "testing"

// The shapes are the ones this installation's sessions actually hold, with the
// content replaced: a fixture that carried a real command or a real account id
// would put a person's work in the repository to prove a classifier works.
func TestOneKindCarriesMoreThanOneMeaning(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Record
		want Category
	}{
		// `usage` is an account's remaining quota in one subtype and what a
		// turn spent in another. Folding them together would let a poll
		// overwrite a turn's cost, or a turn's cost be kept forever as a
		// current value.
		{"claude account quota", Record{Kind: "usage", Text: "get_usage"}, CategoryAccountLimits},
		{"codex account quota", Record{Kind: "usage", Text: "account/rateLimits/updated"}, CategoryAccountLimits},
		{"codex turn tokens", Record{Kind: "usage", Text: "thread/tokenUsage/updated"}, CategoryTurnMetrics},
		{"claude message context", Record{Kind: "usage", Text: "context/message"}, CategoryTurnMetrics},
		{"an unknown usage subtype is still a limit", Record{Kind: "usage", Text: "future/limits"}, CategoryAccountLimits},

		// A tool call is a name and a target next to arguments that the
		// measurement found at 20-27MiB a session. Splitting them is what lets
		// the detail expire while the work it describes stays readable.
		{"call identity", Record{Kind: "tool_call", Field: "item.id"}, CategoryWorkSummary},
		{"call target", Record{Kind: "tool_call", Field: "item.command"}, CategoryWorkSummary},
		{"call arguments", Record{Kind: "tool_call", Field: "item.arguments"}, CategoryWorkDetail},
		{"call patch", Record{Kind: "tool_call", Field: "item.changes"}, CategoryWorkDiff},
		{"result identity", Record{Kind: "tool_result", Field: "tool_use_id"}, CategoryWorkSummary},
		{"result output", Record{Kind: "tool_result", Field: "item.aggregatedOutput"}, CategoryWorkDetail},
		{"result value", Record{Kind: "tool_result", Field: "item.result"}, CategoryWorkDetail},

		// An approval carries a decision, a pending state, and a list of
		// options the provider repeats on every call -- 0.67-0.82MiB a session
		// of the same list.
		{"approval decision", Record{Kind: "approval", Field: "decision_reason"}, CategoryWorkSummary},
		{"approval options", Record{Kind: "approval", Field: "params.availableDecisions"}, CategoryProviderConstant},
		{"approval amendment", Record{Kind: "approval", Field: "params.proposedExecpolicyAmendment"}, CategoryProviderConstant},
		{"approval input", Record{Kind: "approval", Field: "input.content"}, CategoryWorkDetail},

		{"what was said", Record{Kind: "assistant"}, CategoryConversation},
		{"what a person said", Record{Kind: "input"}, CategoryConversation},
		{"a turn's totals", Record{Kind: "turn_end"}, CategoryTurnMetrics},
		{"the catalog", Record{Kind: "models", Text: "catalog"}, CategoryModelCatalog},
		{"whether a poll is in flight", Record{Kind: "usage_status", Text: "polling"}, CategoryAvailability},
		{"what a restart needs", Record{Kind: "receipt"}, CategoryControl},
		{"a pending question", Record{Kind: "permission"}, CategoryControl},
	} {
		if got := Classify(tc.r); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

// Raw is not one thing. Most of what survives retention is the provider's reply
// to cxz's own polling, which is a second copy of what the normalized events
// already say; the rest is the stream that produced the conversation.
func TestRawSeparatesTheStreamFromTheReplies(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want Category
	}{
		{"claude control reply", `{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"success","response":{}}}`, CategoryProviderReply},
		{"claude control request", `{"type":"control_request","request_id":"initialize","request":{}}`, CategoryProviderReply},
		{"codex rpc reply", `{"id":"cxz-quota","result":{"rateLimits":{}}}`, CategoryProviderReply},
		{"codex rpc failure", `{"id":"cxz-models","error":{"code":-32601}}`, CategoryProviderReply},
		{"claude assistant line", `{"type":"assistant","message":{"content":[]}}`, CategoryProviderStream},
		{"codex notification", `{"method":"item/completed","params":{}}`, CategoryProviderStream},
		{"codex delta", `{"method":"item/agentMessage/delta","params":{}}`, CategoryProviderStream},
		// A reply to a notification-shaped method is still the stream: the
		// method is what says this was not asked for by cxz.
		{"method with an id", `{"id":1,"method":"item/started","params":{}}`, CategoryProviderStream},
		{"unreadable", `not json`, CategoryProviderStream},
	} {
		got := Classify(Record{Kind: "raw", RawBody: []byte(tc.body)})
		if got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

// A shed body says nothing about what it held, and the classifier does not
// invent a category from emptiness -- the reader is told `expired` instead.
// There are 271,912 of these in this installation already.
func TestAnEmptyRawIsNotClassifiedByItsEmptiness(t *testing.T) {
	if got := Classify(Record{Kind: "raw"}); got != CategoryProviderStream {
		t.Fatal(got)
	}
}

// A provider that adds an event cxz has never seen must cost a bounded amount
// and stay readable. Deleting it quietly is worse than counting it quietly.
func TestAnUnknownKindIsBoundedRatherThanDropped(t *testing.T) {
	c := Classify(Record{Kind: "something-new", Text: "whatever"})
	if c != CategoryDiagnostic {
		t.Fatal(c)
	}
	if _, ok := (RecordPolicy{}).Bytes(c); !ok {
		t.Fatal("an unknown kind landed in a category with no budget")
	}
	if Default(c).Storage != StorageDisk {
		t.Fatal("an unknown kind is not recorded")
	}
}

// Separate limit windows are separate values. A weekly limit and a five-hour
// limit folded into one key would make the display answer for whichever was
// written last.
func TestLatestKeysKeepWindowsApart(t *testing.T) {
	weekly := LatestKey(CategoryAccountLimits, "acct", "claude", "seven_day")
	hourly := LatestKey(CategoryAccountLimits, "acct", "claude", "five_hour")
	if weekly == hourly {
		t.Fatal(weekly)
	}
	// A caller that has fewer parts does not collide with one that has more.
	if LatestKey(CategoryModelCatalog) == LatestKey(CategoryAccountLimits) {
		t.Fatal("categories share a key")
	}
	if got := LatestKey(CategoryModelCatalog, "", "codex"); got != "model_catalog/codex" {
		t.Fatal(got)
	}
}

func TestOnlyTheConversationWaitsForATurnBoundary(t *testing.T) {
	if !TurnBoundary(CategoryConversation) {
		t.Fatal("the conversation can be trimmed mid-turn")
	}
	for _, c := range []Category{CategoryWorkDetail, CategoryAccountLimits, CategoryModelCatalog, CategoryProviderStream, CategoryDiagnostic} {
		if TurnBoundary(c) {
			t.Fatalf("%s waits for a boundary it does not need", c)
		}
	}
}
