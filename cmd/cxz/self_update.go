package main

import (
	"bytes"
	"context"
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
	return &xli.Command{Name: "self-update", Brief: "Build cxz from Git in Docker; update this executable and the installed local manager",
		Flags: flg.Flags{
			&flg.String{Name: "ref", Brief: "Source branch, tag, or commit in lesomnus/cxz", Default: &ref},
			&flg.Switch{Name: "client-only", Brief: "Update only this executable; skip the local manager", Default: &clientOnly},
		}, Handler: selfUpdateHandler()}
}

func runSelfUpdate(ctx context.Context, c *xli.Command) error {
	ref := flg.MustGet[string](c, "ref")
	if err := selfupdate.ValidateRef(ref); err != nil {
		return err
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
	replacement, err := selfupdate.Prepare(executable)
	if err != nil {
		return err
	}
	defer replacement.Close()
	work, err := os.MkdirTemp("", "cxz-source-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	fmt.Fprintf(c.ErrWriter, "Building cxz from lesomnus/cxz at %s for %s/%s…\n", ref, runtime.GOOS, runtime.GOARCH)
	artifact, err := selfupdate.Build(ctx, work, ref, c.ErrWriter)
	if err != nil {
		return fmt.Errorf("cxz executable unchanged: %w", err)
	}
	if err := replacement.Stage(artifact.Path); err != nil {
		return fmt.Errorf("cxz executable unchanged: %w", err)
	}
	if err := verifyUpdatedExecutable(ctx, replacement.Candidate, artifact.Revision, work); err != nil {
		return fmt.Errorf("cxz executable unchanged: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	changed, err := replacement.Apply()
	if changed {
		fmt.Fprintf(c.Writer, "Updated %s to %s\nPrevious executable: %s\n", replacement.Target, artifact.Revision, replacement.Previous)
	}
	if err != nil {
		if changed {
			return fmt.Errorf("CLI updated, but directory sync failed; manager refresh was skipped: %w", err)
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

func verifyUpdatedExecutable(ctx context.Context, path, revision, work string) error {
	if err := selfupdate.ValidateArtifact(selfupdate.Artifact{Path: path, Revision: revision}); err != nil {
		return err
	}
	// Use an empty state directory: verification must not read user settings,
	// contact a daemon, or mutate an existing installation.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--state", filepath.Join(work, "verify-state"), "version")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("built cxz version check failed: %w: %.2000s", err, output.String())
	}
	if !strings.Contains(output.String(), "source-"+revision[:12]) {
		return fmt.Errorf("built cxz version check did not report the expected source revision")
	}
	return nil
}
