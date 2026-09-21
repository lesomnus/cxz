package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/selfupdate"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func selfUpdateCommand() *xli.Command {
	ref, clientOnly := "main", false
	brief := "Build cxz from Git in Docker; update this executable and the installed local manager"
	refBrief := "Source branch, tag, or commit in lesomnus/cxz"
	if runtime.GOOS == "windows" {
		brief = "Download and update this frontend; no Docker required"
		refBrief = "Published build: main (latest successful CI build) or a release tag"
	}
	return &xli.Command{Name: "self-update", Brief: brief,
		Flags: flg.Flags{
			&flg.String{Name: "ref", Brief: refBrief, Default: &ref},
			&flg.Switch{Name: "client-only", Brief: "Update only this executable; skip the local manager", Default: &clientOnly},
		}, Handler: selfUpdateHandler()}
}

func runSelfUpdate(ctx context.Context, c *xli.Command) error {
	ref := flg.MustGet[string](c, "ref")
	if err := selfupdate.ValidateRef(ref); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := selfupdate.ValidateDownloadRef(ref); err != nil {
			return err
		}
	}
	rootCmd := c
	for rootCmd.HasParent() {
		rootCmd = rootCmd.Parent()
	}
	root, err := filepath.Abs(flg.MustGet[string](rootCmd, "state"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	lock, err := core.Lock(filepath.Join(root, "self-update.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	refreshManager, err := shouldRefreshManager(root, flg.MustGet[bool](c, "client-only"))
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "cxz-self-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	replacement, err := selfupdate.PrepareWithDocker(ctx, executable, work, c.ErrWriter)
	if err != nil {
		return err
	}
	defer replacement.Close()
	var artifact selfupdate.Artifact
	if runtime.GOOS == "windows" {
		artifact, err = selfupdate.DownloadWindows(ctx, work, ref, c.ErrWriter)
	} else {
		fmt.Fprintf(c.ErrWriter, "Building cxz from lesomnus/cxz at %s for %s/%s…\n", ref, runtime.GOOS, runtime.GOARCH)
		artifact, err = selfupdate.Build(ctx, work, ref, c.ErrWriter)
	}
	if err != nil {
		return fmt.Errorf("cxz executable unchanged: %w", err)
	}
	if err := replacement.Stage(artifact.Path); err != nil {
		return fmt.Errorf("cxz executable unchanged: %w", err)
	}
	artifact.Path = replacement.Candidate
	if err := verifyUpdatedExecutable(ctx, artifact, work); err != nil {
		return fmt.Errorf("cxz executable unchanged: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	changed, err := replacement.Apply()
	if changed {
		fmt.Fprintf(c.Writer, "Updated %s to %s (%s)\nPrevious executable: %s\n", replacement.Target, artifact.Version, artifact.Revision, replacement.Previous)
	}
	if err != nil {
		if changed {
			return fmt.Errorf("CLI updated, but installation did not finish cleanly; manager refresh was skipped: %w", err)
		}
		return fmt.Errorf("replace executable: %w", err)
	}
	if !refreshManager {
		fmt.Fprintln(c.Writer, "Local executable updated. No manager refresh requested or installed.")
		return nil
	}
	fmt.Fprintln(c.ErrWriter, "Refreshing the installed local manager; project sessions keep running…")
	// Execute the new binary so its installer and embedded manager Dockerfile are
	// used. Named frontend connections are not installation targets.
	cmd := exec.CommandContext(ctx, replacement.Target, "--state", root, "-x", "install", "--recreate")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.ReadCloser, c.Writer, c.ErrWriter
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local CLI updated, but manager refresh failed; retry cxz --state %q install --recreate: %w", root, err)
	}
	fmt.Fprintln(c.Writer, "Local manager updated. Running project runtimes take the new binary on their next recreate.")
	return nil
}

func shouldRefreshManager(root string, clientOnly bool) (bool, error) {
	if clientOnly || runtime.GOOS != "linux" {
		return false, nil
	}
	_, err := transport.Load(root)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func verifyUpdatedExecutable(ctx context.Context, artifact selfupdate.Artifact, work string) error {
	if err := selfupdate.ValidateArtifact(artifact); err != nil {
		return err
	}
	// Use an empty state directory: verification must not read user settings,
	// contact a daemon, or mutate an existing installation.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{"--state", filepath.Join(work, "verify-state")}
	if runtime.GOOS != "windows" {
		args = append(args, "--format", "json")
	}
	cmd := exec.CommandContext(ctx, artifact.Path, append(args, "version")...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("updated cxz version check failed: %w: %.2000s", err, output.String())
	}
	if !matchesUpdatedVersion(output.Bytes(), artifact.Version) {
		return fmt.Errorf("updated cxz version check did not report the expected version %s", artifact.Version)
	}
	return nil
}

func matchesUpdatedVersion(output []byte, expected string) bool {
	var value struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(output, &value) == nil {
		return value.Version == expected
	}
	fields := strings.Fields(string(output))
	return len(fields) >= 2 && fields[0] == "cxz" && fields[1] == expected
}
