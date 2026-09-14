package wisp

import (
	"encoding/hex"
	"fmt"
)

const HostSecretsMount = "/cxz/host-secrets"

func HostSecretRoot(owner string) (string, error) {
	if len(owner) < 8 || len(owner) > 128 {
		return "", fmt.Errorf("invalid secret storage owner")
	}
	if _, err := hex.DecodeString(owner); err != nil {
		return "", fmt.Errorf("invalid secret storage owner")
	}
	return "/dev/shm/cxz-" + owner, nil
}
