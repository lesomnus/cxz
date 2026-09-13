package distribution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func ValidVersion(v string) bool { return len(v) < 40 && versionPattern.MatchString(v) }
func SelectedVersion(root, kind, fallback string) string {
	var versions map[string]string
	b, err := os.ReadFile(filepath.Join(root, "releases.json"))
	if err == nil && json.Unmarshal(b, &versions) == nil && ValidVersion(versions[kind]) {
		return versions[kind]
	}
	return fallback
}

func Latest(ctx context.Context, kind string) (string, error) {
	url := "https://downloads.claude.ai/claude-code-releases/latest"
	if kind == "codex" {
		url = "https://github.com/openai/codex/releases/latest"
	} else if kind == "gh" {
		url = "https://github.com/cli/cli/releases/latest"
	} else if kind != "claude" {
		return "", fmt.Errorf("unknown release provider")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "cxz-release-check")
	r, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return "", fmt.Errorf("release check %s: HTTP %d", kind, r.StatusCode)
	}
	if kind != "claude" {
		// GitHub's latest stable-release redirect avoids the shared unauthenticated
		// API rate limit. Require its canonical tag route, not arbitrary HTML.
		prefix := strings.TrimSuffix(req.URL.Path, "latest") + "tag/"
		if r.Request.URL.Host != "github.com" || !strings.HasPrefix(r.Request.URL.Path, prefix) {
			return "", fmt.Errorf("latest release did not resolve to a stable tag")
		}
		v := strings.TrimPrefix(r.Request.URL.Path, prefix)
		if kind == "codex" {
			v = strings.TrimPrefix(v, "rust-v")
		} else {
			v = strings.TrimPrefix(v, "v")
		}
		if !ValidVersion(v) {
			return "", fmt.Errorf("invalid stable version from %s", kind)
		}
		return v, nil
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if !ValidVersion(v) {
		return "", fmt.Errorf("invalid stable version from %s", kind)
	}
	return v, nil
}
