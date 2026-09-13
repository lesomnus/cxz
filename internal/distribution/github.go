package distribution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
)

const GitHubVersion = "2.100.0"

func EnsureGitHub(ctx context.Context, root, arch string) (string, error) {
	return EnsureGitHubVersion(ctx, root, arch, SelectedVersion(root, "gh", GitHubVersion))
}
func EnsureGitHubVersion(ctx context.Context, root, arch, version string) (string, error) {
	if !ValidVersion(version) {
		return "", fmt.Errorf("invalid gh release version")
	}
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported gh architecture %s", arch)
	}
	name := "gh_" + version + "_linux_" + arch
	dir := filepath.Join(root, "gh", version, arch)
	bin := filepath.Join(dir, name, "bin", "gh")
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return "", err
	}
	lock, err := core.Lock(dir + ".lock")
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if _, err = os.Stat(bin); err == nil {
		return bin, nil
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".gh-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	base := "https://github.com/cli/cli/releases/download/v" + version + "/"
	sums := filepath.Join(tmp, "checksums.txt")
	if err = get(ctx, base+"gh_"+version+"_checksums.txt", sums); err != nil {
		return "", err
	}
	b, err := os.ReadFile(sums)
	if err != nil {
		return "", err
	}
	sum := ""
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name+".tar.gz" {
			sum = f[0]
		}
	}
	if sum == "" {
		return "", fmt.Errorf("gh release checksum missing")
	}
	archive := filepath.Join(tmp, "gh.tar.gz")
	if err = get(ctx, base+name+".tar.gz", archive); err != nil {
		return "", err
	}
	if err = verify(archive, sum); err != nil {
		return "", err
	}
	if err = unpack(archive, tmp); err != nil {
		return "", err
	}
	if err = os.Chmod(filepath.Join(tmp, name, "bin", "gh"), 0755); err != nil {
		return "", err
	}
	if err = os.Chmod(tmp, 0755); err != nil {
		return "", err
	}
	if err = os.Rename(tmp, dir); err != nil {
		return "", err
	}
	return bin, core.SyncDir(filepath.Dir(dir))
}
