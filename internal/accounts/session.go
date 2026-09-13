package accounts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
)

// SessionRoot is stable across login/Create retries and container recreation.
// Hash the opaque creation key rather than accepting it as a filesystem path.
// Credentials are never copied from another session or the old project profile.
func SessionRoot(root, creationKey string) string {
	hash := sha256.Sum256([]byte(creationKey))
	return filepath.Join(root, "session-profiles", fmt.Sprintf("%x", hash[:16]))
}

func AuthRoot(root string, s core.Session) string {
	if s.AuthBackend == ProjectLocalOAuth {
		return SessionRoot(root, s.CreateID)
	}
	return root
}

func LaunchSession(root string, s core.Session, env []string) (LaunchAuth, error) {
	b, err := ResolveBinding(s.Kind, s.AuthBackend, s.ProjectID, s.Account, s.AuthBinding)
	if err != nil {
		return LaunchAuth{}, err
	}
	if s.CreateID == "" {
		return LaunchAuth{}, fmt.Errorf("session creation key required")
	}
	profile := SessionRoot(root, s.CreateID)
	if err := Prepare(profile, s.Account, s.Kind); err != nil {
		return LaunchAuth{}, err
	}
	auth, err := b.Launch(AuthRoot(root, s), s.Account, env)
	if err != nil {
		return LaunchAuth{}, err
	}
	// Broker capabilities stay project-scoped; app-server state does not.
	if s.AuthBackend == BrokeredAccessToken {
		if _, err := os.Lstat(filepath.Join(Config(profile, s.Account), "auth.json")); !os.IsNotExist(err) {
			return LaunchAuth{}, fmt.Errorf("brokered session must not contain auth.json")
		}
		auth.Env = Environment(env, profile, s.Account, s.Kind)
		auth.ConfigDir = Config(profile, s.Account)
	}
	return auth, nil
}
