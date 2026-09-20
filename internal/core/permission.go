package core

// AutomaticApproval is the supported set of permission requests. Questions
// and unknown provider methods always need an explicit reply.
func AutomaticApproval(name string) bool {
	switch name {
	case "Bash", "Read", "Edit", "Write", "Glob", "Grep", "WebSearch", "WebFetch", "ToolSearch", "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
		return true
	}
	return false
}
