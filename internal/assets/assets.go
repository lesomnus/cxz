// Package assets publishes named hard links to flob content-addressed files.
//
// What an upload *is* lives here, and the store that publishes one lives in
// store.go, which a page does not build: the sandbox serves the same raw HTTP
// handler the Manager serves, and that handler needs this contract without
// needing a content-addressed host filesystem behind it.
package assets

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxSize int64 = 1024 * 1024 * 1024
const MountPath = "/cxz/assets"

var ErrSizeMismatch = errors.New("attachment size mismatch")

func ValidName(name string) bool {
	return name != "" && len(name) <= 255 && utf8.ValidString(name) && name != "." && name != ".." && !strings.ContainsAny(name, "/\\") && !strings.ContainsFunc(name, unicode.IsControl)
}

// Upload describes a single live request. It is never persisted as metadata.
type Upload struct {
	SessionID, RunID, Name string
	Size                   int64
}

type Uploader interface {
	UploadAttachment(context.Context, Upload, io.Reader) (string, error)
}

func ValidID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
