package workspace

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/wisp"
	"os"
	"path/filepath"
	"time"
)

func (m *Manager) prepareSecretRoot(p *Project) (string, error) {
	host, err := wisp.HostSecretRoot(m.Owner)
	if err != nil {
		return "", err
	}
	if p.ID == "" || filepath.Base(p.ID) != p.ID || p.ID == "." || p.ID == ".." {
		return "", fmt.Errorf("invalid project secret directory")
	}
	if err = wisp.CheckSecretRoot(wisp.HostSecretsMount); err != nil {
		return "", fmt.Errorf("host secret tmpfs unavailable; update/recreate the cxz manager: %w", err)
	}
	if err = os.Chmod(wisp.HostSecretsMount, 0700); err != nil {
		return "", err
	}
	local := filepath.Join(wisp.HostSecretsMount, p.ID)
	if err = os.Mkdir(local, os.ModeSticky|0777); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err = wisp.CheckSecretRoot(local); err != nil {
		return "", err
	}
	if err = os.Chmod(local, os.ModeSticky|0777); err != nil {
		return "", err
	}
	return filepath.Join(host, p.ID), nil
}

// Host tmpfs survives all cxz processes. Sweep at manager startup and hourly,
// including orphaned project directories, without reading secret contents.
func (m *Manager) RunSecretSweep(ctx context.Context) {
	sweep := func() {
		if wisp.CheckSecretRoot(wisp.HostSecretsMount) != nil {
			return
		}
		entries, err := os.ReadDir(wisp.HostSecretsMount)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				_ = wisp.SweepSecrets(filepath.Join(wisp.HostSecretsMount, entry.Name()), time.Now())
			}
		}
	}
	sweep()
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			sweep()
		}
	}
}
