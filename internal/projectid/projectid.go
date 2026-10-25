// Package projectid defines the project UUID and the legacy-reference bridge.
package projectid

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/lesomnus/payday/pdid"
)

// New allocates an identity independently of the workspace path.
func New() string { return pdid.New(7).Uuid().String() }

// Canonical preserves UUIDs and maps pre-UUID handles to their existing resource
// UUID. This is the same mapping previously used by lifecycle.resourceID.
func Canonical(id string) string {
	if id == "" {
		return ""
	}
	if v, err := uuid.Parse(id); err == nil {
		return v.String()
	}
	h := sha256.Sum256([]byte("cxz.resource:" + id))
	var v uuid.UUID
	copy(v[:], h[:16])
	v[6] = (v[6] & 15) | 0x80
	v[8] = (v[8] & 63) | 0x80
	v[9] = 7
	return v.String()
}

func Bytes(id string) []byte {
	if id == "" {
		return nil
	}
	v := uuid.MustParse(Canonical(id))
	return v[:]
}

func String(id []byte) string {
	v, err := uuid.FromBytes(id)
	if err != nil {
		return ""
	}
	return v.String()
}

func Same(a, b string) bool { return a != "" && b != "" && Canonical(a) == Canonical(b) }

func Legacy(id string) bool {
	if len(id) != 24 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
