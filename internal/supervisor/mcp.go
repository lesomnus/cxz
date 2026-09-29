package supervisor

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"sort"
	"strconv"
)

func mcpArgs(args []string, kind, exe, root, id string, snapshot mcpconfig.Snapshot) []string {
	servers := map[string]any{}
	ids := make([]string, 0, len(snapshot.Servers))
	for id := range snapshot.Servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, name := range ids {
		s := snapshot.Servers[name]
		commandArgs := []string{"--state", root, "_mcp-bridge", id, name}
		if kind == "codex" {
			prefix := "mcp_servers." + name + "."
			if s.Kind == "http" {
				args = append(args, "-c", prefix+"url="+strconv.Quote(s.URL))
				if len(s.Headers) > 0 {
					keys := make([]string, 0, len(s.Headers))
					for k := range s.Headers {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						args = append(args, "-c", prefix+"http_headers."+strconv.Quote(k)+"="+strconv.Quote(s.Headers[k]))
					}
				}
			} else {
				b, _ := json.Marshal(commandArgs)
				args = append(args, "-c", prefix+"command="+strconv.Quote(exe), "-c", prefix+"args="+string(b))
			}
		} else {
			if s.Kind == "http" {
				servers[name] = map[string]any{"type": "http", "url": s.URL, "headers": s.Headers}
			} else {
				servers[name] = map[string]any{"type": "stdio", "command": exe, "args": commandArgs}
			}
		}
	}
	if kind != "codex" {
		b, _ := json.Marshal(map[string]any{"mcpServers": servers})
		for i := range args {
			if args[i] == "--mcp-config" && i+1 < len(args) {
				args[i+1] = string(b)
			}
		}
	}
	return args
}
