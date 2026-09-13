package installer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/githubauth"
	"github.com/lesomnus/cxz/internal/transport"
)

// Only the active credential for each configured host is exported. Neither
// command errors nor YAML errors may include secret-bearing output.
func githubSnapshot(ctx context.Context) ([]byte, error) {
	dir := os.Getenv("GH_CONFIG_DIR")
	if dir == "" {
		dir = os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			dir = filepath.Join(home, ".config")
		}
		dir = filepath.Join(dir, "gh")
	}
	hosts := map[string]map[string]any{}
	b, err := os.ReadFile(filepath.Join(dir, "hosts.yml"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot read host gh configuration")
	}
	if len(b) > 1<<20 {
		return nil, fmt.Errorf("host gh configuration exceeds 1 MiB")
	}
	if len(b) > 0 {
		if yaml.Unmarshal(b, &hosts) != nil {
			return nil, fmt.Errorf("invalid host gh configuration")
		}
	}
	if hosts == nil {
		hosts = map[string]map[string]any{}
	}
	if os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" {
		if hosts["github.com"] == nil {
			hosts["github.com"] = map[string]any{}
		}
	}
	if host := os.Getenv("GH_HOST"); host != "" {
		if hosts[host] == nil {
			hosts[host] = map[string]any{}
		}
	}
	result := map[string]map[string]string{}
	gh, _ := exec.LookPath("gh")
	for host, config := range hosts {
		if strings.ContainsAny(host, "/\\\r\n\x00 ") || strings.HasPrefix(host, "-") || host == "" {
			return nil, fmt.Errorf("invalid gh host name")
		}
		token, _ := config["oauth_token"].(string)
		envToken := ""
		keys := []string{"GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}
		if host == "github.com" || strings.HasSuffix(host, ".ghe.com") {
			keys = []string{"GH_TOKEN", "GITHUB_TOKEN"}
		}
		for _, key := range keys {
			if value := os.Getenv(key); value != "" {
				envToken = value
				break
			}
		}
		if envToken != "" {
			token = envToken
		} else if gh != "" {
			call, cancel := context.WithTimeout(ctx, 5*time.Second)
			cmd := exec.CommandContext(call, gh, "auth", "token", "--hostname", host)
			// Deliberately discard stderr; gh diagnostics can contain secrets.
			value, e := cmd.Output()
			cancel()
			if e == nil {
				token = strings.TrimSpace(string(value))
			} else {
				return nil, fmt.Errorf("cannot export host gh credential; unlock the keychain or log in with gh")
			}
		}
		if token == "" {
			return nil, fmt.Errorf("host gh credential is in a keychain; install gh on the host to export it")
		}
		if strings.ContainsAny(token, "\r\n\x00") || len(token) > 65536 {
			return nil, fmt.Errorf("invalid host gh token")
		}
		user, _ := config["user"].(string)
		protocol, _ := config["git_protocol"].(string)
		if protocol != "ssh" {
			protocol = "https"
		}
		result[host] = map[string]string{"oauth_token": token, "git_protocol": protocol, "user": user}
	}
	return yaml.Marshal(result)
}

// Snapshot travels through stdin, not Docker argv/env or a host bind mount.
// The daemon's owned state volume is private; projects receive their own copy.
func SyncGitHub(ctx context.Context, root string, out io.Writer, projects ...*api.Project) error {
	v, err := transport.Load(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	b, err := githubSnapshot(ctx)
	if err != nil {
		return err
	}
	c, err := dockerx.Inspect(ctx, v.Container)
	if err != nil {
		return err
	}
	if c.Config.Labels["cxz.owner"] != v.Owner || c.Config.Labels["cxz.role"] != "daemon" {
		return fmt.Errorf("refusing gh sync to unowned manager")
	}
	script := `set -eu
umask 077
mkdir -p /var/lib/cxz/host-gh
test ! -L /var/lib/cxz/host-gh
chmod 700 /var/lib/cxz/host-gh
tmp=$(mktemp /var/lib/cxz/host-gh/.hosts.XXXXXX)
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
mv -f "$tmp" /var/lib/cxz/host-gh/hosts.yml`
	if err = dockerx.Input(ctx, bytes.NewReader(b), "exec", "-i", v.Container, "sh", "-c", script); err != nil {
		return fmt.Errorf("could not sync host gh credentials to manager")
	}
	for _, p := range projects {
		if _, err = dockerx.Owned(ctx, p.ContainerId, v.Owner, p.Id); err != nil {
			return err
		}
		user := p.RemoteUser
		if user == "" {
			user = "root"
		}
		if err = githubauth.Inject(ctx, p.ContainerId, user, b); err != nil {
			return err
		}
	}
	if out != nil {
		fmt.Fprintln(out, "cxz: host gh credentials synced (project provisioning applies them)")
	}
	return nil
}

func readyWithGitHub(ctx context.Context, root string, v transport.Installation, out io.Writer) error {
	if err := waitReady(ctx, v, out); err != nil {
		return err
	}
	return SyncGitHub(ctx, root, out)
}
