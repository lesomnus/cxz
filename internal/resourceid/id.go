package resourceid

import "crypto/sha256"

// Bytes preserves a resource's identity when a runtime inventory is rebuilt.
func Bytes(domain byte, runtimeID string) []byte {
	h := sha256.Sum256([]byte("cxz.resource:" + runtimeID))
	id := append([]byte(nil), h[:16]...)
	id[6] = (id[6] & 15) | 0x80
	id[8] = (id[8] & 63) | 0x80
	id[9] = domain
	return id
}
