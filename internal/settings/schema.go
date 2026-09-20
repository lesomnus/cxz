package settings

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
)

const (
	SchemaFilename  = "settings.schema.json"
	SchemaReference = "./" + SchemaFilename
)

//go:embed settings.schema.json
var schemaDocument []byte

// EnsureSchema refreshes the editor schema from this binary. It needs no daemon
// or network connection; an identical file is left untouched.
func EnsureSchema(root string) error {
	path := filepath.Join(root, SchemaFilename)
	previous, err := os.ReadFile(path)
	if err == nil && bytes.Equal(previous, schemaDocument) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return core.WriteFile(path, schemaDocument)
}
