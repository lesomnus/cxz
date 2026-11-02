package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func (m *Manager) EraseAccount(ctx context.Context, alias, agent, backend string) error {
	if err := accounts.Validate(alias, agent); err != nil {
		return err
	}
	if _, err := accounts.Resolve(agent, backend); err != nil {
		return err
	}
	if m.Owner == "" {
		return fmt.Errorf("account erasure requires an installed Manager")
	}
	controller, err := m.auxiliaryController()
	if err != nil {
		return err
	}
	config := controller.Config()
	for _, item := range []struct {
		kind    string
		profile auxiliary.Profile
	}{
		{"summary", config.Summary}, {"suggestion", config.Suggestion}, {"title", config.Title},
	} {
		if item.profile.Account == alias {
			if _, err := controller.Save(item.kind, auxiliary.Profile{}); err != nil {
				return err
			}
		}
	}
	// Save cancels old jobs before waiting for their private execution volume.
	lock := m.projectLock("auxiliary:" + alias)
	lock.Lock()
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := accounts.EraseCentral(ctx, m.Root, alias); err != nil {
		return err
	}
	volume := "cxz-" + m.Owner + "-aux-" + alias
	names, err := dockerx.Run(ctx, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	found := false
	for _, name := range strings.Fields(string(names)) {
		found = found || name == volume
	}
	if !found {
		return nil
	}
	raw, err := dockerx.Run(ctx, "volume", "inspect", volume)
	if err != nil {
		return err
	}
	var resources []struct{ Labels map[string]string }
	if json.Unmarshal(raw, &resources) != nil || len(resources) != 1 || resources[0].Labels["cxz.owner"] != m.Owner || resources[0].Labels["cxz.project"] != "auxiliary:"+alias {
		return fmt.Errorf("refusing to erase an unowned account authentication volume")
	}
	_, err = dockerx.Run(ctx, "volume", "rm", volume)
	return err
}
