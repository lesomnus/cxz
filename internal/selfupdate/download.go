package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const releaseAPI = "https://api.github.com/repos/lesomnus/cxz/releases/tags/"
const releaseDownloads = "https://github.com/lesomnus/cxz/releases/download/"
const maxArchiveSize = 128 << 20
const maxExecutableSize = 512 << 20

var releaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$`)

func ValidateDownloadRef(ref string) error {
	if ref != "main" && !releaseTag.MatchString(ref) {
		return fmt.Errorf("Windows updates download published builds: use --ref main or a release tag such as vX.Y.Z; arbitrary source refs require a build on a Linux host")
	}
	return nil
}

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

type releaseInfo struct {
	Tag    string         `json:"tag_name"`
	Draft  bool           `json:"draft"`
	Assets []releaseAsset `json:"assets"`
}

// DownloadWindows uses only HTTPS and Go's ZIP support. The frontend never
// needs Docker, Git, Go, PowerShell, or an authenticated GitHub CLI to update.
func DownloadWindows(ctx context.Context, work, ref string, out io.Writer) (Artifact, error) {
	return downloadWindows(ctx, &http.Client{Timeout: 10 * time.Minute}, work, ref, runtime.GOARCH, out)
}

func downloadWindows(ctx context.Context, client *http.Client, work, ref, arch string, out io.Writer) (Artifact, error) {
	if err := ValidateDownloadRef(ref); err != nil {
		return Artifact{}, err
	}
	if arch != "amd64" && arch != "arm64" {
		return Artifact{}, fmt.Errorf("no published Windows build for %s", arch)
	}
	tag := ref
	if tag == "main" {
		tag = "edge"
	}
	fmt.Fprintf(out, "Downloading cxz %s for windows/%s from GitHub…\n", ref, arch)
	metadata, err := fetchReleaseFile(ctx, client, releaseAPI+url.PathEscape(tag), 2<<20)
	if err != nil {
		return Artifact{}, fmt.Errorf("published build %q unavailable (main requires a successful CI publication): %w", ref, err)
	}
	var release releaseInfo
	if err := json.Unmarshal(metadata, &release); err != nil {
		return Artifact{}, fmt.Errorf("invalid release metadata: %w", err)
	}
	if release.Draft || release.Tag != tag {
		return Artifact{}, fmt.Errorf("release metadata does not match published tag %q", tag)
	}
	name := "cxz-" + tag + "-windows-" + arch + ".zip"
	archive, err := findReleaseAsset(release, name)
	if err != nil {
		return Artifact{}, err
	}
	sumsAsset, err := findReleaseAsset(release, "SHA256SUMS")
	if err != nil {
		return Artifact{}, err
	}
	sums, err := fetchReleaseFile(ctx, client, sumsAsset.URL, 64<<10)
	if err != nil {
		return Artifact{}, err
	}
	if err := checkAssetDigest(sums, sumsAsset.Digest); err != nil {
		return Artifact{}, err
	}
	expected, err := archiveChecksum(sums, name)
	if err != nil {
		return Artifact{}, err
	}
	data, err := fetchReleaseFile(ctx, client, archive.URL, maxArchiveSize)
	if err != nil {
		return Artifact{}, err
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != expected {
		return Artifact{}, fmt.Errorf("downloaded archive checksum differs; existing executable kept (retry if a new build was being published)")
	}
	if err := checkAssetDigest(data, archive.Digest); err != nil {
		return Artifact{}, err
	}
	path := filepath.Join(work, "cxz.exe")
	if err := unpackWindowsArchive(data, path); err != nil {
		return Artifact{}, err
	}
	a := Artifact{Path: path, Version: tag}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return Artifact{}, fmt.Errorf("read downloaded executable: %w", err)
	}
	for _, value := range info.Settings {
		if value.Key == "vcs.revision" {
			a.Revision = value.Value
		}
	}
	// ValidateArtifact verifies module, native platform and clean build metadata
	// before the command executes the candidate or replaces the installed image.
	if err := ValidateArtifact(a); err != nil {
		return Artifact{}, err
	}
	if tag == "edge" {
		a.Version = "source-" + a.Revision[:12]
	}
	return a, nil
}

func findReleaseAsset(release releaseInfo, name string) (releaseAsset, error) {
	var found *releaseAsset
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		if found != nil {
			return releaseAsset{}, fmt.Errorf("duplicate release asset %q", name)
		}
		expected := releaseDownloads + url.PathEscape(release.Tag) + "/" + name
		if asset.URL != expected {
			return releaseAsset{}, fmt.Errorf("unexpected download location for %q", name)
		}
		found = &asset
	}
	if found == nil {
		return releaseAsset{}, fmt.Errorf("release %s has no %s; existing executable kept", release.Tag, name)
	}
	return *found, nil
}

func fetchReleaseFile(ctx context.Context, client *http.Client, location string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cxz-self-update")
	if strings.HasPrefix(location, releaseAPI) {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	// Preserve the caller's transport, but reject a downgrade on asset redirects.
	copy := *client
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 10 {
			return fmt.Errorf("unsafe or excessive download redirects")
		}
		return nil
	}
	res, err := copy.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub download: %s", res.Status)
	}
	if res.ContentLength > limit {
		return nil, fmt.Errorf("download exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeds size limit")
	}
	return data, nil
}

func checkAssetDigest(data []byte, digest string) error {
	// Older releases predate GitHub's digest field. SHA256SUMS is always checked.
	if digest == "" {
		return nil
	}
	sum := sha256.Sum256(data)
	if digest != "sha256:"+hex.EncodeToString(sum[:]) {
		return fmt.Errorf("release asset changed since metadata was read; retry the update")
	}
	return nil
}

func archiveChecksum(sums []byte, name string) (string, error) {
	var found string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		value, err := hex.DecodeString(fields[0])
		if err != nil || len(value) != sha256.Size || found != "" {
			return "", fmt.Errorf("invalid or duplicate checksum for %s", name)
		}
		found = hex.EncodeToString(value)
	}
	if found == "" {
		return "", fmt.Errorf("missing checksum for %s", name)
	}
	return found, nil
}

func unpackWindowsArchive(data []byte, path string) (err error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("invalid Windows archive: %w", err)
	}
	if len(r.File) != 1 || r.File[0].Name != "cxz.exe" || !r.File[0].Mode().IsRegular() ||
		r.File[0].UncompressedSize64 == 0 || r.File[0].UncompressedSize64 > maxExecutableSize {
		return fmt.Errorf("Windows archive must contain only a regular cxz.exe within the size limit")
	}
	in, err := r.File[0].Open()
	if err != nil {
		return err
	}
	defer in.Close()
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	defer func() {
		output.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	size, err := io.Copy(output, io.LimitReader(in, maxExecutableSize+1))
	if err != nil {
		return err
	}
	if size > maxExecutableSize {
		return fmt.Errorf("extracted executable exceeds size limit")
	}
	if err := output.Sync(); err != nil {
		return err
	}
	return output.Close()
}
