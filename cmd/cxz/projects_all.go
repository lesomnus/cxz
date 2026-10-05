//go:build !windows

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

// allProjects is the one WORKSPACE that is not a workspace. It stays in the
// argument rather than becoming a flag because WORKSPACE defaults to ".", so a
// --all flag would always arrive beside a target it then has to ignore; here
// the argument remains the single statement of what to replace. Nothing can
// shadow it: no path begins with "@", and no alias may contain one.
//
// "@" is the spelling cxz already uses for a selector that is not a name, as in
// cxz use @edge and @stable.
const allProjects = "@all"

// isAllProjects also accepts "!all", which reads the same but has to be quoted:
// interactive bash and zsh expand a leading "!" as a history event, so an
// unquoted !all never reaches the program ("event not found: all"). Taking it
// anyway costs one comparison and saves the person who typed it unquoted,
// learned to quote it, and now has it in a script.
func isAllProjects(handle string) bool { return handle == allProjects || handle == "!all" }

// recreateAllProjects replaces the container of every owned project that is
// running, and prepares them without opening sessions -- recreating a dozen
// projects should not start a dozen agents. Projects that are stopped or have
// no container are left alone rather than started: they read the current
// configuration when they next come up, and nothing that was down comes up as
// a side effect of this.
//
// One project failing is not the others' failure. Each is attempted, each
// failure is named, and the command still ends in an error so a script does not
// read a partial pass as a pass.
func recreateAllProjects(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	// These name one project's settings or one session's. Applied to every
	// project at once they would either be wrong everywhere or silently ignored.
	for _, flag := range []string{"config", "name", "alias", "agent", "model", "account"} {
		if value, set := flg.Get[string](c, flag); set && value != "" {
			return fmt.Errorf("--%s applies to one project; recreate %s takes only --yes and --trust-config", flag, allProjects)
		}
	}
	list, err := client.Projects(ctx, &api.Empty{})
	if err != nil {
		return err
	}
	var targets []*api.Project
	var idle, foreign int
	for _, p := range list.Projects {
		switch {
		case p.State == "foreign":
			foreign++ // Another owner's container is not ours to replace.
		case p.State != "running":
			idle++ // absent or stopped: nothing to replace, and not ours to start.
		default:
			targets = append(targets, p)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Workspace < targets[j].Workspace })
	skipped := []string{}
	if idle > 0 {
		skipped = append(skipped, fmt.Sprintf("%d not running", idle))
	}
	if foreign > 0 {
		skipped = append(skipped, fmt.Sprintf("%d foreign", foreign))
	}
	if len(targets) == 0 {
		fmt.Fprintf(c.ErrWriter, "No owned project is running%s. Bring one up with cxz up WORKSPACE.\n", suffix(skipped))
		return nil
	}
	if !flg.MustGet[bool](c, "yes") {
		fmt.Fprintln(c.ErrWriter, "Recreate removes the writable layer and disconnects attached editors. Workspace and named volumes are kept.")
		for _, p := range targets {
			fmt.Fprintf(c.ErrWriter, "Target: %s %s (%s)\n", p.ContainerId, p.Workspace, p.State)
		}
		fmt.Fprintf(c.ErrWriter, "%d projects%s. Sessions are not started.\n", len(targets), suffix(skipped))
		return fmt.Errorf("review targets with cxz project ls, then pass --yes")
	}
	if _, err = syncDevcontainer(ctx, client, stateFrom(ctx), settings.From(ctx)); err != nil {
		return err
	}
	resources := client.(*resourceclient.Client)
	trust := flg.MustGet[bool](c, "trust-config")
	var failed []string
	for i, p := range targets {
		fmt.Fprintf(c.ErrWriter, "cxz: recreating %s (%d/%d); image or agent downloads may take a few minutes\n", p.Workspace, i+1, len(targets))
		request := &api.ProjectRequest{Workspace: p.Workspace, Recreate: true, Confirmed: true, TrustConfig: trust, PrepareOnly: true, ClientId: core.ID()}
		call, cancel := context.WithTimeout(ctx, 30*time.Minute)
		_, err = resources.Open(call, request)
		cancel()
		if err != nil {
			fmt.Fprintf(c.ErrWriter, "cxz: %s was not recreated: %v\n", p.Workspace, err)
			failed = append(failed, p.Workspace)
			continue
		}
		fmt.Fprintf(c.Writer, "recreated %s %s\n", p.Id, p.Workspace)
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d projects were not recreated: %s; retry one with cxz project recreate --yes PROJECT", len(failed), len(targets), strings.Join(failed, ", "))
	}
	fmt.Fprintf(c.ErrWriter, "cxz: recreated %d projects\n", len(targets))
	return nil
}

func suffix(skipped []string) string {
	if len(skipped) == 0 {
		return ""
	}
	return " (skipped " + strings.Join(skipped, ", ") + ")"
}
