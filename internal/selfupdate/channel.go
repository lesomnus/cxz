package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/lesomnus/cxz/internal/releasechannel"
	"io"
	"os"
	"path/filepath"
)

func DownloadChannel(ctx context.Context, work, goos, arch string, r releasechannel.Release, out io.Writer) (Artifact, error) {
	if e := r.Validate(); e != nil {
		return Artifact{}, e
	}
	platform := goos + "/" + arch
	asset, ok := r.Assets[platform]
	if !ok {
		return Artifact{}, fmt.Errorf("unsupported channel platform %s", platform)
	}
	path := filepath.Join(work, "cxz")
	if goos == "windows" {
		path += ".exe"
	}
	if cached, err := os.ReadFile(path); err == nil {
		sum := sha256.Sum256(cached)
		if hex.EncodeToString(sum[:]) == asset.SHA256 {
			a := Artifact{Path: path, Version: r.Version, Revision: r.Revision}
			return a, ValidatePlatform(a, goos, arch)
		}
	}
	fmt.Fprintf(out, "Downloading %s (%s) for %s…\n", r.Version, r.Revision, platform)
	b, e := releasechannel.Fetch(ctx, releasechannel.DownloadBase+r.Tag+"/"+asset.Name, maxExecutableSize)
	if e != nil {
		return Artifact{}, e
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != asset.SHA256 {
		return Artifact{}, fmt.Errorf("channel executable checksum mismatch")
	}
	if e = os.WriteFile(path, b, 0700); e != nil {
		return Artifact{}, e
	}
	a := Artifact{Path: path, Version: r.Version, Revision: r.Revision}
	return a, ValidatePlatform(a, goos, arch)
}
