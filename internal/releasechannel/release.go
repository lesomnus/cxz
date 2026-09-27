// Package releasechannel resolves published channels to immutable release snapshots.
package releasechannel

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/mod/semver"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const DownloadBase = "https://github.com/lesomnus/cxz/releases/download/"
const APIBase = "https://api.github.com/repos/lesomnus/cxz/releases"

var revision = regexp.MustCompile(`^[a-f0-9]{40}$`)
var checksum = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}
type Release struct {
	Version   string           `json:"version"`
	Tag       string           `json:"tag"`
	Revision  string           `json:"revision"`
	Ancestors []string         `json:"ancestors,omitempty"`
	Sequence  int64            `json:"sequence"`
	Protocol  int              `json:"protocol"`
	Schema    int              `json:"schema"`
	Image     string           `json:"image"`
	Assets    map[string]Asset `json:"assets"`
}

func StableTag(tag string) bool {
	return semver.IsValid(tag) && semver.Canonical(tag) == tag && semver.Prerelease(tag) == ""
}
func (r Release) Validate() error {
	if !revision.MatchString(r.Revision) || r.Sequence <= 0 || r.Protocol != 1 || r.Schema != 1 {
		return fmt.Errorf("invalid or incompatible release identity")
	}
	if r.Tag != "edge" && !StableTag(r.Tag) {
		return fmt.Errorf("invalid release tag")
	}
	if r.Tag == "edge" {
		if r.Version != "source-"+r.Revision[:12] {
			return fmt.Errorf("edge version does not match revision")
		}
	} else if r.Version != r.Tag {
		return fmt.Errorf("stable version does not match tag")
	}
	if !strings.HasPrefix(r.Image, "ghcr.io/lesomnus/cxz@sha256:") || !checksum.MatchString(strings.TrimPrefix(r.Image, "ghcr.io/lesomnus/cxz@sha256:")) {
		return fmt.Errorf("manager image must be pinned by digest")
	}
	for _, v := range r.Ancestors {
		if !revision.MatchString(v) {
			return fmt.Errorf("invalid ancestor")
		}
	}
	for _, platform := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		a := r.Assets[platform]
		name := "cxz-" + r.Revision + "-" + strings.ReplaceAll(platform, "/", "-")
		if strings.HasPrefix(platform, "windows/") {
			name += ".exe"
		}
		if a.Name != name || !checksum.MatchString(a.SHA256) {
			return fmt.Errorf("invalid release asset for %s", platform)
		}
	}
	return nil
}
func Fetch(ctx context.Context, location string, limit int64) ([]byte, error) {
	return fetch(ctx, &http.Client{Timeout: 5 * time.Minute}, location, limit)
}
func fetch(ctx context.Context, client *http.Client, location string, limit int64) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", location, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "cxz-release-channel")
	c := *client
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || len(via) >= 10 {
			return fmt.Errorf("unsafe release redirect")
		}
		return nil
	}
	resp, e := c.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("release request: HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e == nil && int64(len(b)) > limit {
		e = fmt.Errorf("release response too large")
	}
	return b, e
}
func LatestStable(ctx context.Context) (string, error) {
	return latestStable(ctx, &http.Client{Timeout: time.Minute})
}
func latestStable(ctx context.Context, c *http.Client) (string, error) {
	best := ""
	for page := 1; page <= 100; page++ {
		b, e := fetch(ctx, c, fmt.Sprintf("%s?per_page=100&page=%d", APIBase, page), 8<<20)
		if e != nil {
			return "", e
		}
		var rows []struct {
			Tag        string `json:"tag_name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		if e = json.Unmarshal(b, &rows); e != nil {
			return "", e
		}
		for _, r := range rows {
			if !r.Draft && !r.Prerelease && StableTag(r.Tag) && semver.Compare(r.Tag, best) > 0 {
				best = r.Tag
			}
		}
		if len(rows) < 100 {
			if best == "" {
				return "", fmt.Errorf("no stable release published")
			}
			return best, nil
		}
	}
	return "", fmt.Errorf("release listing exceeds supported limit")
}
func Resolve(ctx context.Context, channel string) (Release, error) {
	tag := "edge"
	if channel == "stable" {
		var e error
		tag, e = LatestStable(ctx)
		if e != nil {
			return Release{}, e
		}
	} else if channel != "edge" {
		return Release{}, fmt.Errorf("unknown channel %q", channel)
	}
	b, e := Fetch(ctx, DownloadBase+tag+"/cxz-update.json", 1<<20)
	if e != nil {
		return Release{}, fmt.Errorf("%s has no complete channel publication: %w", tag, e)
	}
	var r Release
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	if r.Tag != tag {
		return r, fmt.Errorf("release manifest tag mismatch")
	}
	return r, r.Validate()
}
