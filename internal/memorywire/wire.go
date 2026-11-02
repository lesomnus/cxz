// Package memorywire carries typed MemoryService calls through a short-lived
// retained-volume helper when a project runtime is stopped. It is not a public RPC.
package memorywire

import (
	"encoding/json"
	"github.com/lesomnus/cxz/api"
)

type Request struct {
	Project *api.Project
	Method  string
	Data    json.RawMessage
}
type Reply struct {
	Data  json.RawMessage
	Code  int32
	Error string
}
