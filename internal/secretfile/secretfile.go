// Package secretfile carries the wire shape of a redacted secret between a
// client that cannot reach the engine and the manager that can. It holds no
// logic, so a frontend does not have to link the manager to ask for a file.
package secretfile

// Action is "put" or "delete". The secret itself travels only on a put, and
// only over a connection that keeps it off the network; see transport.
type Request struct {
	Action  string `json:"action"`
	Project string `json:"project"`
	Session string `json:"session,omitempty"`
	Path    string `json:"path,omitempty"`
	Secret  []byte `json:"secret,omitempty"`
}
type Reply struct {
	Path string `json:"path"`
}
