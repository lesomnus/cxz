//go:build !linux

package wisp

import (
	"fmt"
	"time"
)

const MaxSecretBytes = 64 * 1024
const DefaultSecretMaxIdle = 8 * time.Hour

func CheckSecretRoot(string) error         { return fmt.Errorf("secret tmpfs requires Linux") }
func SweepSecrets(string, time.Time) error { return fmt.Errorf("secret tmpfs requires Linux") }

type secretStore struct{}

func (s *secretStore) clear(session string) {}
func (s *secretStore) handle(r Request) Response {
	clear(r.Secret)
	return Response{Done: true, Error: "secret tmpfs requires Linux"}
}
