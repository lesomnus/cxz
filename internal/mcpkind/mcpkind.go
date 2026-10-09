// Package mcpkind translates what an MCP connection is between the enum the
// resource API speaks and the name the runtime API and the config store use.
//
// The two surfaces name the same thing differently on purpose, the same way
// auxkind does: a public API should refuse a kind it does not know, which an
// enum does by itself, and the runtime view models have always carried kinds as
// strings.
package mcpkind

import "github.com/lesomnus/cxz/resource"

const (
	Stdio = "stdio"
	HTTP  = "http"
	// Builtin is provided by cxz. It is never registered by a caller, so it
	// only ever travels outwards.
	Builtin = "builtin"
)

var names = map[resource.McpKind]string{
	resource.McpKind_MCP_KIND_STDIO:   Stdio,
	resource.McpKind_MCP_KIND_HTTP:    HTTP,
	resource.McpKind_MCP_KIND_BUILTIN: Builtin,
}

// Name is the empty string for a kind this build does not know, so a
// definition from a newer client is refused by validation rather than stored as
// something it is not.
func Name(k resource.McpKind) string { return names[k] }

func Of(name string) resource.McpKind {
	for k, n := range names {
		if n == name {
			return k
		}
	}
	return resource.McpKind_MCP_KIND_UNSPECIFIED
}
