package tui

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/engine"
	"github.com/lesomnus/cxz/internal/historypolicy"
)

// engineAction names the call behind each button. The label is still a string
// because it is a label; what it reaches is a method.
func engineAction(ctx context.Context, client api.SessionsClient, action string) (*api.EngineReply, error) {
	switch action {
	case "up":
		return client.StartEngine(ctx, &api.StartEngineInput{})
	case "down":
		return client.StopEngine(ctx, &api.Empty{})
	case "prune":
		return client.PruneEngine(ctx, &api.Empty{})
	}
	return nil, fmt.Errorf("unknown engine action: %s", action)
}

// readSettingsInfo asks the three questions the page shows. The engine's
// failing is what makes the status unavailable; a budget or a build that cannot
// be read is reported in its own field, as it was when one reply carried all
// three.
func readSettingsInfo(ctx context.Context, client api.SessionsClient) (settingsInfo, error) {
	v, err := client.GetEngineInfo(ctx, &api.Empty{})
	if err != nil {
		return settingsInfo{}, err
	}
	out := settingsInfo{Info: engine.Info{
		Mode: v.Mode, State: v.State, Health: v.Health,
		Image: v.Image, ConfiguredImage: v.ConfiguredImage, Endpoint: v.Endpoint,
		BuildCache: v.BuildCache, Reclaimable: v.Reclaimable, UsageError: v.UsageError,
	}}
	if policy, err := client.GetHistoryPolicy(ctx, &api.Empty{}); err != nil {
		out.HistoryError = err.Error()
	} else {
		out.History = &historypolicy.Policy{
			Disabled: policy.Disabled, MaxMiB: int(policy.MaxMib), RawMiB: int(policy.RawMib),
			WindowMiB: int(policy.WindowMib), WindowTurns: int(policy.WindowTurns),
		}
	}
	if build, err := client.GetInstallationVersion(ctx, &api.Empty{}); err != nil {
		out.CXZError = err.Error()
	} else {
		out.CXZVersion, out.CXZRevision = build.Version, build.Revision
		out.CXZChannel, out.CXZPin, out.CXZError = build.Channel, build.Pin, build.Error
	}
	return out, nil
}
