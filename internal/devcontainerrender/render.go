// Package devcontainerrender carries the wire shape of a devcontainer
// inspection between the manager that reads the files and the client that
// writes them out. It holds no logic, so the client does not have to link the
// manager to ask the question.
package devcontainerrender

const MaxBytes = 1 << 20

// ResolvedCompose names the merged Compose configuration in a reply. The client
// prints that one file on request, so both sides have to agree on its name.
const ResolvedCompose = "compose/resolved.yaml"

type Request struct {
	Project string `json:"project"`
}

// File is one file as it will be laid out in the output directory. Role is one
// line of prose, because the order of a merge is the thing a reader needs and
// the thing a file name cannot say.
type File struct {
	Name   string `json:"name"`   // path inside the output directory
	Source string `json:"source"` // absolute path on the manager host
	Role   string `json:"role"`
	Data   []byte `json:"data"`
}
type Reply struct {
	Project   string `json:"project"`
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
	Note      string `json:"note,omitempty"`
	Files     []File `json:"files"`
}
