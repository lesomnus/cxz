// Package devcontainerrender holds the two facts about a devcontainer
// inspection that both the manager reading the files and the client writing
// them out have to agree on. The reply itself is a generated message now, so
// this is no longer a wire shape -- only the bound and the one file name a
// client looks for by name.
package devcontainerrender

const MaxBytes = 1 << 20

// ResolvedCompose names the merged Compose configuration in a reply. The client
// prints that one file on request, so both sides have to agree on its name.
const ResolvedCompose = "compose/resolved.yaml"
