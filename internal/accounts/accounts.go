// Package accounts owns private vendor authentication files. No secret is a
// resource field: the manager transfers only the selected profile over stdin.
package accounts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
)

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func Validate(alias, agent string) error {
	if !aliasPattern.MatchString(alias) {
		return fmt.Errorf("account alias must be 1–63 lowercase letters, digits or hyphens, starting with a letter")
	}
	if agent != "codex" && agent != "claude" {
		return fmt.Errorf("account agent must be codex or claude")
	}
	return nil
}
func Dir(root, alias string) string    { return filepath.Join(root, "accounts", alias) }
func Config(root, alias string) string { return filepath.Join(Dir(root, alias), "config") }
func filename(agent string) string {
	if agent == "codex" {
		return "auth.json"
	}
	return ".credentials.json"
}
func Prepare(root, alias, agent string) error {
	if err := Validate(alias, agent); err != nil {
		return err
	}
	for _, p := range []string{Dir(root, alias), Config(root, alias), filepath.Join(Dir(root, alias), "home")} {
		if err := os.MkdirAll(p, 0700); err != nil {
			return err
		}
		st, err := os.Lstat(p)
		if err != nil || !st.IsDir() {
			return fmt.Errorf("account directory is not a real directory")
		}
		if err := os.Chmod(p, 0700); err != nil {
			return err
		}
	}
	return nil
}
func Credential(root, alias, agent string) ([]byte, error) {
	if err := Validate(alias, agent); err != nil {
		return nil, err
	}
	path := filepath.Join(Config(root, alias), filename(agent))
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 1024*1024 {
		return nil, fmt.Errorf("account %s needs login; run cxz account login %s", alias, alias)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read credentials for account %s", alias)
	}
	if err := validateCredential(alias, agent, b); err != nil {
		return nil, err
	}
	return b, nil
}
func validateCredential(alias, agent string, b []byte) error {
	var v map[string]json.RawMessage
	if json.Unmarshal(b, &v) != nil {
		return fmt.Errorf("invalid credential file for account %s", alias)
	}
	// Subscription OAuth profiles only. API keys and environmental providers are
	// intentionally not an implicit alternative to the selected login.
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if agent == "codex" {
		var key string
		_ = json.Unmarshal(v["OPENAI_API_KEY"], &key)
		if key != "" {
			return fmt.Errorf("account %s must use subscription login, not an API key", alias)
		}
		if json.Unmarshal(v["tokens"], &token) != nil || token.AccessToken == "" {
			return fmt.Errorf("account %s needs Codex subscription login", alias)
		}
	} else {
		var oauth struct {
			AccessToken string `json:"accessToken"`
		}
		if json.Unmarshal(v["claudeAiOauth"], &oauth) != nil || oauth.AccessToken == "" {
			return fmt.Errorf("account %s needs Claude subscription login", alias)
		}
	}
	return nil
}

// Install does not overwrite refreshed project tokens until the manager login
// changes. Vendor history/config remains in this account's project directory.
func Install(root, alias, agent string, credential []byte) error {
	if err := Validate(alias, agent); err != nil {
		return err
	}
	if err := validateCredential(alias, agent, credential); err != nil {
		return err
	}
	if err := Prepare(root, alias, agent); err != nil {
		return err
	}
	sum := sha256.Sum256(credential)
	revision := hex.EncodeToString(sum[:])
	marker := filepath.Join(Dir(root, alias), "source.json")
	var old string
	if b, err := os.ReadFile(marker); err == nil {
		_ = json.Unmarshal(b, &old)
	}
	if old == revision {
		if _, err := Credential(root, alias, agent); err == nil {
			return nil
		}
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(credential, &data) != nil {
		return fmt.Errorf("invalid credential transfer")
	}
	if err := core.WriteJSON(filepath.Join(Config(root, alias), filename(agent)), data); err != nil {
		return err
	}
	if _, err := Credential(root, alias, agent); err != nil {
		return err
	}
	return core.WriteJSON(marker, revision)
}

// Environment prevents accidental host/project credentials or vendor config
// from overriding Account. This is not a sandbox against the same OS user.
func Environment(env []string, root, alias, agent string) []string {
	var out []string
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		blocked := key == "HOME" || key == "XDG_CONFIG_HOME" || key == "XDG_DATA_HOME" || key == "XDG_CACHE_HOME" || key == "XDG_STATE_HOME" || key == "XDG_RUNTIME_DIR" || key == "DBUS_SESSION_BUS_ADDRESS"
		for _, prefix := range []string{"OPENAI_", "ANTHROPIC_", "CLAUDE_", "CLAUDECODE", "CODEX_", "AZURE_", "AWS_", "GOOGLE_", "GCLOUD_", "GEMINI_"} {
			blocked = blocked || strings.HasPrefix(key, prefix)
		}
		if !blocked {
			out = append(out, entry)
		}
	}
	home := filepath.Join(Dir(root, alias), "home")
	out = append(out, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+filepath.Join(home, ".local/share"), "XDG_CACHE_HOME="+filepath.Join(home, ".cache"), "XDG_STATE_HOME="+filepath.Join(home, ".local/state"))
	if agent == "codex" {
		out = append(out, "CODEX_HOME="+Config(root, alias))
	} else {
		out = append(out, "CLAUDE_CONFIG_DIR="+Config(root, alias))
	}
	return out
}
