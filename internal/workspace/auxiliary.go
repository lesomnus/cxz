package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
	"io"
	"os/exec"
	"time"
)

func (m *Manager) auxiliaryController() (*auxiliary.Controller, error) {
	m.auxMu.Lock()
	defer m.auxMu.Unlock()
	if m.aux == nil {
		var e error
		m.aux, e = auxiliary.New(m.Root, m.runAuxiliary)
		if e != nil {
			return nil, e
		}
	}
	return m.aux, nil
}
func (m *Manager) auxiliaryRequest(ctx context.Context, b []byte) (*api.Receipt, error) {
	var r auxiliary.Request
	if len(b) > 16384 {
		return nil, fmt.Errorf("auxiliary request too large")
	}
	if e := json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	c, e := m.auxiliaryController()
	if e != nil {
		return nil, e
	}
	out := auxiliary.Reply{Config: c.Config()}
	switch r.Action {
	case "", "list":
	case "put":
		if r.Profile.Enabled {
			o, e := m.runAuxiliary(ctx, auxiliary.Input{Profile: r.Profile, Task: "models"})
			if e != nil {
				return nil, e
			}
			if e = auxiliary.ValidateModel(r.Profile, o.Models); e != nil {
				return nil, e
			}
		}
		old := out.Config
		out.Config, e = c.Save(r.Task, r.Profile)
		if e == nil {
			for _, p := range []auxiliary.Profile{old.Summary, old.Suggestion} {
				if !p.Enabled || p.Backend != accounts.BrokeredAccessToken {
					continue
				}
				needed := false
				for _, next := range []auxiliary.Profile{out.Config.Summary, out.Config.Suggestion} {
					needed = needed || (next.Enabled && next.Account == p.Account)
				}
				if !needed {
					if err := accounts.RevokeAuxiliaryGrant(m.Root, p.Account); err != nil {
						out.Message = "Settings saved; auxiliary grant cleanup failed: " + err.Error()
					}
				}
			}
		}
	case "models":
		var o auxiliary.Output
		o, e = m.runAuxiliary(ctx, auxiliary.Input{Profile: r.Profile, Task: "models"})
		out.Models = o.Models
	case "status":
		out.Job, e = c.Status(r.Session)
	case "forget":
		e = c.Forget(r.Session)
	case "cancel":
		e = c.Cancel(r.Session)
		out.Job, _ = c.Status(r.Session)
	case "login-info":
		out.Profile = &r.Profile
		out.Owner = m.Owner
	default:
		return nil, fmt.Errorf("unknown auxiliary action")
	}
	if e != nil {
		return nil, e
	}
	b, e = json.Marshal(out)
	return &api.Receipt{Status: string(b)}, e
}
func (m *Manager) auxiliaryArgs(ctx context.Context, p auxiliary.Profile) ([]string, string, error) {
	if e := accounts.Validate(p.Account, p.Agent); e != nil {
		return nil, "", e
	}
	if _, e := accounts.Resolve(p.Agent, p.Backend); e != nil {
		return nil, "", e
	}
	if m.Owner == "" || m.ToolsVolume == "" || m.Image == "" {
		return nil, "", fmt.Errorf("auxiliary execution requires an installed Manager")
	}
	bin, e := distribution.Ensure(ctx, "/cxz/tools", p.Agent, "", false)
	if e != nil {
		return nil, "", e
	}
	volume := "cxz-" + m.Owner + "-aux-" + p.Account
	if e = dockerx.EnsureResource(ctx, "volume", volume, m.Owner, "auxiliary:"+p.Account); e != nil {
		return nil, "", e
	}
	// Only dedicated auth and immutable tools. No workspace, project state or Docker socket.
	name := "cxz-aux-" + core.ID()
	args := []string{"run", "--rm", "-i", "--name", name, "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "1g", "--tmpfs", "/tmp:rw,nosuid,nodev,size=256m", "--label", "cxz.owner=" + m.Owner, "--label", "cxz.auxiliary=true", "--mount", "type=volume,source=" + m.ToolsVolume + ",target=/cxz/tools,readonly", "--mount", "type=volume,source=" + volume + ",target=/cxz/aux", "--entrypoint", "/cxz/tools/cxz", m.Image, "--state", "/cxz/aux"}
	return args, bin, nil
}
func (m *Manager) runAuxiliary(ctx context.Context, in auxiliary.Input) (auxiliary.Output, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	lock := m.projectLock("auxiliary:" + in.Profile.Account)
	lock.Lock()
	defer lock.Unlock()
	if e := ctx.Err(); e != nil {
		return auxiliary.Output{}, e
	}
	args, bin, e := m.auxiliaryArgs(ctx, in.Profile)
	if e != nil {
		return auxiliary.Output{}, e
	}
	q := auxiliary.HelperInput{Input: in, Binary: bin}
	if in.Profile.Backend == accounts.BrokeredAccessToken {
		g, e := accounts.IssueAuxiliaryGrant(m.Root, in.Profile.Account)
		if e != nil {
			return auxiliary.Output{}, e
		}
		q.Grant = &g
	}
	b, _ := json.Marshal(q)
	var output limitedAuxBuffer
	var diagnostic limitedAuxBuffer
	e = runAuxProcess(ctx, append(args, "_auxiliary-job"), bytes.NewReader(b), &output, &diagnostic)
	if e != nil {
		return auxiliary.Output{}, fmt.Errorf("auxiliary execution failed: %w: %s", e, diagnostic.String())
	}
	var out auxiliary.Output
	e = json.Unmarshal(output.Bytes(), &out)
	return out, e
}
func (m *Manager) AuxiliaryLogin(ctx context.Context, p auxiliary.Profile, r io.Reader, w, errw io.Writer) error {
	args, bin, e := m.auxiliaryArgs(ctx, p)
	if e != nil {
		return e
	}
	return runAuxProcess(ctx, append(args, "_auxiliary-login", p.Account, p.Agent, p.Backend, bin), r, w, errw)
}
func runAuxProcess(ctx context.Context, args []string, r io.Reader, w, errw io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = r
	cmd.Stdout = w
	cmd.Stderr = errw
	cmd.WaitDelay = 2 * time.Second
	e := cmd.Run()
	if ctx.Err() != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i, a := range args {
			if a == "--name" && i+1 < len(args) {
				_, _ = dockerx.Run(cleanup, "rm", "-f", args[i+1])
				break
			}
		}
	}
	return e
}

type limitedAuxBuffer struct{ bytes.Buffer }

func (b *limitedAuxBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 128<<10 {
		return 0, fmt.Errorf("auxiliary helper output exceeded limit")
	}
	return b.Buffer.Write(p)
}

// Observe independently of frontend connections. Only new turns since task
// activation are eligible; first discovery reads a bounded recent event window.
func (m *Manager) StartAuxiliary(ctx context.Context) {
	_, _ = m.auxiliaryController()
	ctx, cancel := context.WithCancel(ctx)
	m.auxStop = cancel
	m.auxDone = make(chan struct{})
	go func() {
		defer close(m.auxDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		cursors := map[string]uint64{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			c, e := m.auxiliaryController()
			if e != nil || !c.Config().Active() {
				continue
			}
			projects, e := m.all(ctx)
			if e != nil {
				continue
			}
			alive := map[string]bool{}
			for _, p := range projects {
				if p.ContainerID == "" {
					continue
				}
				q, done := context.WithTimeout(ctx, 3*time.Second)
				conn, client, e := m.client(q, p)
				if e != nil {
					done()
					continue
				}
				list, e := client.List(q, &api.Empty{})
				if e == nil {
					for _, s := range list.Sessions {
						alive[s.Id] = true
						cursor, ok := cursors[s.Id]
						if !ok {
							cursor = c.Cursor(s.Id)
							if cursor == 0 && s.LastSeq > 512 {
								cursor = s.LastSeq - 512
							}
						}
						if s.LastSeq <= cursor {
							continue
						}
						// At most four pages per scan, so a noisy session cannot monopolize polling.
						for page := 0; page < 4; page++ {
							batch, e := client.History(q, &api.WatchRequest{SessionId: s.Id, AfterSeq: cursor})
							if e != nil || len(batch.Events) == 0 {
								break
							}
							c.Observe(batch.Events)
							next := batch.Events[len(batch.Events)-1].Seq
							if next <= cursor {
								break
							}
							cursor = next
						}
						cursors[s.Id] = cursor
					}
				}
				conn.Close()
				done()
			}
			for id := range cursors {
				if !alive[id] {
					delete(cursors, id)
				}
			}
		}
	}()
}
