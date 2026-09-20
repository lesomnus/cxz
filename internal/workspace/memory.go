package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/memoryview"
)

type memoryMount struct {
	volume, project, target string
	write                   bool
}

func (m *Manager) memoryHelper(ctx context.Context, project, entry string, input any, mounts []memoryMount) ([]byte, error) {
	mounts = append(mounts, memoryMount{volume: m.ToolsVolume, target: "/cxz/tools"})
	name := "cxz-memory-" + core.ID()
	args := []string{"run", "--rm", "-i", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--cap-add", "DAC_OVERRIDE", "--cap-add", "CHOWN", "--security-opt", "no-new-privileges", "--label", "cxz.owner=" + m.Owner, "--label", "cxz.project=" + project}
	for _, v := range mounts {
		// Reads must never recreate a missing or adopt a foreign volume.
		b, err := dockerx.Run(ctx, "volume", "inspect", v.volume)
		if err != nil {
			return nil, fmt.Errorf("retained storage unavailable: %w", err)
		}
		var volumes []struct{ Labels map[string]string }
		if json.Unmarshal(b, &volumes) != nil || len(volumes) != 1 || volumes[0].Labels["cxz.owner"] != m.Owner || volumes[0].Labels["cxz.project"] != v.project {
			return nil, fmt.Errorf("refusing unowned retained storage")
		}
		mount := "type=volume,source=" + v.volume + ",target=" + v.target
		if !v.write {
			mount += ",readonly"
		}
		args = append(args, "--mount", mount)
	}
	args = append(args, "--entrypoint", "/cxz/tools/cxz", m.Image, "--state", "/cxz/state/data", entry)
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = dockerx.Run(cleanup, "rm", "-f", name)
		}
		return nil, fmt.Errorf("retained agent data unavailable: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
func memorySession(s *api.Session) core.Session {
	return core.Session{ID: s.Id, CreateID: s.CreateId, Kind: s.Agent, Account: s.Account}
}
func findMemorySession(projects []*Project, id string) (*Project, *api.Session, error) {
	for _, p := range projects {
		for _, s := range p.Sessions {
			if s.Id == id {
				return p, s, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("session not found: %s", id)
}
func (m *Manager) Memory(ctx context.Context, r *api.MemoryRequest) (*api.MemoryReply, error) {
	projects, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	p, s, err := findMemorySession(projects, r.SessionId)
	if err != nil {
		return nil, err
	}
	out, err := m.memoryHelper(ctx, p.ID, "_memory-read", memoryview.Query{Session: memorySession(s), Path: r.Path}, []memoryMount{{volume: p.Volume, project: p.ID, target: "/cxz/state"}})
	if err != nil {
		return nil, err
	}
	var page memoryview.Page
	if err = json.Unmarshal(out, &page); err != nil {
		return nil, err
	}
	page.Location = "volume:" + p.Volume + strings.TrimPrefix(page.Location, "/cxz/state")
	out, err = json.Marshal(page)
	return &api.MemoryReply{Data: out}, err
}
func (m *Manager) CopyMemory(ctx context.Context, r *api.CopyMemoryRequest) (*api.Receipt, error) {
	projects, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	source, s, err := findMemorySession(projects, r.SessionId)
	if err != nil {
		return nil, err
	}
	target, t, err := findMemorySession(projects, r.TargetId)
	if err != nil {
		return nil, err
	}
	lock := m.projectLock(target.ID)
	lock.Lock()
	defer lock.Unlock()
	out, err := m.memoryHelper(ctx, target.ID, "_memory-copy", memoryview.CopyQuery{Source: memorySession(s), Target: memorySession(t), Path: r.Path, TargetPath: r.TargetPath},
		[]memoryMount{{volume: source.Volume, project: source.ID, target: "/cxz/state"}, {volume: target.Volume, project: target.ID, target: "/cxz/target", write: true}})
	if err != nil {
		return nil, err
	}
	var message string
	if err = json.Unmarshal(out, &message); err != nil {
		return nil, err
	}
	return &api.Receipt{Status: message}, nil
}
