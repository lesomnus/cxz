//go:build !linux

package wisp

const MaxSecretBytes = 64 * 1024

type secretStore struct{}

func (s *secretStore) clear(session string) {}
func (s *secretStore) handle(r Request) Response {
	clear(r.Secret)
	return Response{Done: true, Error: "secret tmpfs requires Linux"}
}
