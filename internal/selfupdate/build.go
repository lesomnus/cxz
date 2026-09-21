// Package selfupdate builds cxz in an isolated Docker checkout and replaces the
// local executable. It does not connect to cxz daemons or recreate projects.
package selfupdate

import (
	"context"
	"debug/buildinfo"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/lesomnus/cxz/internal/core"
)

//go:embed build.Dockerfile
var dockerfile string

var validRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,199}$`)
var validRevision = regexp.MustCompile(`^[0-9a-f]{40}$`)

func ValidateRef(ref string) error {
	if !validRef.MatchString(ref) {
		return fmt.Errorf("ref must be a branch, tag, or commit without spaces or a leading hyphen")
	}
	return nil
}

type Artifact struct {
	Path, Revision string
}

// Build exports to the Docker client's filesystem, including with a remote
// Docker daemon. No host bind mount, Docker socket, or privileged build is used.
func Build(ctx context.Context, work, ref string, out io.Writer) (Artifact, error) {
	return build(ctx, work, ref, out, func(cmd *exec.Cmd) error { return cmd.Run() })
}

func build(ctx context.Context, work, ref string, out io.Writer, run func(*exec.Cmd) error) (Artifact, error) {
	if err := ValidateRef(ref); err != nil {
		return Artifact{}, err
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return Artifact{}, fmt.Errorf("source updates support Linux and Windows")
	}
	name := "cxz"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.CommandContext(ctx, "docker", "buildx", "build", "--progress=plain",
		"--build-arg", "CXZ_REF="+ref, "--build-arg", "CXZ_FETCH="+core.ID(),
		"--build-arg", "CXZ_GOOS="+runtime.GOOS, "--build-arg", "CXZ_GOARCH="+runtime.GOARCH,
		"--build-arg", "CXZ_BINARY="+name, "--output", "type=local,dest=out", "-f", "-", ".")
	cmd.Dir = work
	cmd.Stdin = strings.NewReader(dockerfile)
	cmd.Stdout, cmd.Stderr = out, out
	if err := run(cmd); err != nil {
		return Artifact{}, fmt.Errorf("source build failed (Docker with Buildx and a Linux builder is required): %w", err)
	}
	revision, err := os.ReadFile(filepath.Join(work, "out", "revision"))
	if err != nil {
		return Artifact{}, err
	}
	a := Artifact{Path: filepath.Join(work, "out", name), Revision: strings.TrimSpace(string(revision))}
	if !validRevision.MatchString(a.Revision) {
		return Artifact{}, fmt.Errorf("build returned an invalid source revision")
	}
	if err := ValidateArtifact(a); err != nil {
		return Artifact{}, err
	}
	return a, nil
}

func ValidateArtifact(a Artifact) error {
	info, err := buildinfo.ReadFile(a.Path)
	if err != nil {
		return fmt.Errorf("read built executable: %w", err)
	}
	if info.Path != "github.com/lesomnus/cxz/cmd/cxz" || info.Main.Path != "github.com/lesomnus/cxz" {
		return fmt.Errorf("build output is not a cxz executable")
	}
	values := map[string]string{}
	for _, v := range info.Settings {
		values[v.Key] = v.Value
	}
	if values["GOOS"] != runtime.GOOS || values["GOARCH"] != runtime.GOARCH {
		return fmt.Errorf("built executable platform differs from %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if !validRevision.MatchString(a.Revision) || values["vcs.revision"] != a.Revision || values["vcs.modified"] != "false" {
		return fmt.Errorf("built executable does not match the clean source revision %s", a.Revision)
	}
	return nil
}
