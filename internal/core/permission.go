package core

// CodexAsyncQuestion is a cxz request type, not a provider JSON-RPC method.
const CodexAsyncQuestion = "agentMessage/questions"

// Question reports whether a pending request asks the user for content rather
// than for a decision. A decision has two answers and a policy can hold one of
// them; an answer is content, and nothing about the session implies it.
func Question(name string) bool {
	switch name {
	case "AskUserQuestion", "item/tool/requestUserInput", CodexAsyncQuestion:
		return true
	}
	return false
}

// AutomaticApproval reports whether full mode may answer a request by itself.
// Full is full: every permission request is approved, whichever tool or method
// carries it. The container is the trust boundary that makes the mode
// defensible, not the name of the tool — and an allowlist only moves the prompt
// to whatever the provider ships next, which is the friction the mode exists to
// remove. Questions are the one exception, and not as a policy: see Question.
func AutomaticApproval(name string) bool { return !Question(name) }
