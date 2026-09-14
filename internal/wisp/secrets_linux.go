//go:build linux

package wisp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"sync"
	"time"
)

const SecretRoot = "/cxz/secrets"
const MaxSecretBytes = 64 * 1024
const DefaultSecretMaxIdle = 8 * time.Hour

type secretFile struct {
	session, name string
	root, dir     int
}
type secretStore struct {
	mu    sync.Mutex
	files map[string]*secretFile
}

func secretRoot() (int, error) {
	if os.Getenv("CXZ_SECRET_STORAGE") != "host-tmpfs" {
		return -1, fmt.Errorf("host secret mount unavailable; recreate project with the updated manager")
	}
	return checkedSecretRoot(SecretRoot)
}

func checkedSecretRoot(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("secret tmpfs unavailable; recreate project with /cxz/secrets tmpfs")
	}
	var fs unix.Statfs_t
	if unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.TMPFS_MAGIC || fs.Flags&(unix.ST_NODEV|unix.ST_NOSUID|unix.ST_NOEXEC) != (unix.ST_NODEV|unix.ST_NOSUID|unix.ST_NOEXEC) {
		unix.Close(fd)
		return -1, fmt.Errorf("secret storage must be tmpfs with nodev,nosuid,noexec; recreate project")
	}
	return fd, nil
}

func (s *secretStore) put(session string, body []byte) (string, error) {
	if session == "" || len(session) > 256 || len(body) == 0 || len(body) > MaxSecretBytes {
		return "", fmt.Errorf("invalid secret size or session")
	}
	root, err := secretRoot()
	if err != nil {
		return "", err
	}
	var name string
	for attempt := 0; attempt < 64; attempt++ {
		var random [4]byte
		if _, err = rand.Read(random[:]); err != nil {
			break
		}
		name = hex.EncodeToString(random[:])
		err = unix.Mkdirat(root, name, 0700)
		if err != unix.EEXIST {
			break
		}
	}
	if err != nil {
		unix.Close(root)
		return "", fmt.Errorf("secret allocation failed")
	}
	dir, err := unix.Openat(root, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Unlinkat(root, name, unix.AT_REMOVEDIR)
		unix.Close(root)
		return "", fmt.Errorf("secret allocation failed")
	}
	f := &secretFile{session: session, name: name, root: root, dir: dir}
	fd, err := unix.Openat(dir, "value", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err == nil {
		remaining := body
		for len(remaining) > 0 {
			var n int
			n, err = unix.Write(fd, remaining)
			if err != nil || n == 0 {
				break
			}
			remaining = remaining[n:]
		}
		if len(remaining) > 0 && err == nil {
			err = fmt.Errorf("short write")
		}
		unix.Close(fd)
	}
	if err != nil {
		f.remove()
		return "", fmt.Errorf("secret write failed")
	}
	path := SecretRoot + "/" + name + "/value"
	s.track(path, f)
	return path, nil
}

func (s *secretStore) track(path string, f *secretFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = map[string]*secretFile{}
	}
	s.files[path] = f
}
func (f *secretFile) remove() {
	unix.Unlinkat(f.dir, "value", 0)
	unix.Close(f.dir)
	unix.Unlinkat(f.root, f.name, unix.AT_REMOVEDIR)
	unix.Close(f.root)
}
func (s *secretStore) drop(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.files[path]; f != nil {
		delete(s.files, path)
		f.remove()
	}
}
func (s *secretStore) clear(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for path, f := range s.files {
		if session == "" || f.session == session {
			delete(s.files, path)
			unix.Close(f.dir)
			unix.Close(f.root)
		}
	}
}
func (s *secretStore) handle(r Request) Response {
	switch r.Operation {
	case "secret/check":
		fd, err := secretRoot()
		if err != nil {
			return Response{Done: true, Error: err.Error()}
		}
		unix.Close(fd)
		return Response{Done: true}
	case "secret/put":
		p, err := s.put(r.Session, r.Secret)
		clear(r.Secret)
		if err != nil {
			return Response{Done: true, Error: err.Error()}
		}
		return Response{Done: true, SecretPath: p}
	case "secret/delete":
		s.drop(r.Path)
		return Response{Done: true}
	case "secret/clear":
		if r.Session == "" {
			return Response{Done: true, Error: "session required"}
		}
		s.clear(r.Session)
		return Response{Done: true}
	default:
		return Response{Done: true, Error: "unknown wisp operation"}
	}
}
