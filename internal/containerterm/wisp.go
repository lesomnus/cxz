package containerterm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/logview"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/wisp"
)

// WispPool owns one stdio helper per project/container/user while attached.
// Requests are serialized per helper; cancelled lookups are drained so that
// their remaining frames cannot become the next request's response.
type WispPool struct {
	mu          sync.Mutex
	clients     map[string]*wispClient
	diagnostics map[string]*logview.Buffer
}
type wispClient struct {
	secrets      bool
	secretPolicy string
	gate         chan struct{}
	enc          *json.Encoder
	dec          *json.Decoder
	stop         context.CancelFunc
	done         chan struct{}
}

func (p *WispPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		c.stop()
	}
	p.clients = nil
}

func (p *WispPool) client(lifetime, ctx context.Context, project *api.Project) (*wispClient, error) {
	if project == nil || project.Id == "" || project.ContainerId == "" || project.RemoteUser == "" {
		return nil, fmt.Errorf("project container unavailable")
	}
	key := project.Id + "/" + project.ContainerId + "/" + project.RemoteUser
	p.mu.Lock()
	if p.clients == nil {
		p.clients = make(map[string]*wispClient)
	}
	c := p.clients[key]
	if c != nil {
		select {
		case <-c.done:
			delete(p.clients, key)
			c = nil
		default:
		}
	}
	var err error
	if c == nil {
		// A recreated container must not retain its predecessor's helper.
		for k, old := range p.clients {
			if len(k) > len(project.Id) && k[:len(project.Id)+1] == project.Id+"/" {
				old.stop()
				delete(p.clients, k)
			}
		}
		if p.diagnostics == nil {
			p.diagnostics = map[string]*logview.Buffer{}
		}
		log := p.diagnostics[project.Id]
		if log == nil {
			log = &logview.Buffer{}
			p.diagnostics[project.Id] = log
		}
		fmt.Fprintf(log, "%s starting workspace helper\n", time.Now().Format(time.RFC3339))
		c, err = openWisp(lifetime, ctx, project, log)
		if err != nil {
			fmt.Fprintf(log, "wisp start failed: %v\n", err)
		}
		if err == nil {
			p.clients[key] = c
		}
	}
	p.mu.Unlock()
	return c, err
}

func (p *WispPool) Paths(lifetime, ctx context.Context, project *api.Project, dir string, emit func(PathListing)) (PathListing, error) {
	c, err := p.client(lifetime, ctx, project)
	if err != nil {
		return PathListing{}, err
	}
	select {
	case <-ctx.Done():
		return PathListing{}, ctx.Err()
	case <-c.done:
		return PathListing{}, fmt.Errorf("wisp disconnected; retry path lookup")
	case c.gate <- struct{}{}:
	}
	// The bounded directory limit also bounds this queue if the consumer cancels.
	frames := make(chan wisp.Response, 2050)
	go func() {
		defer func() { <-c.gate; close(frames) }()
		if err := c.enc.Encode(wisp.Request{Path: dir}); err != nil {
			c.stop()
			return
		}
		for {
			var r wisp.Response
			if err := c.dec.Decode(&r); err != nil {
				c.stop()
				return
			}
			frames <- r
			if r.Done {
				return
			}
		}
	}()
	var out PathListing
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				c.stop()
			}
			return out, ctx.Err()
		case r, ok := <-frames:
			if !ok {
				return out, fmt.Errorf("wisp disconnected; update project runtime if wisp is unavailable")
			}
			out.Entries = append(out.Entries, r.Entries...)
			out.Truncated = r.Truncated
			if r.Error != "" {
				return out, fmt.Errorf("wisp: %s", r.Error)
			}
			if r.Done {
				return out, nil
			}
			if emit != nil && (len(out.Entries) == 1 || len(out.Entries)%16 == 0) {
				emit(PathListing{Entries: append([]PathEntry(nil), out.Entries...)})
			}
		}
	}
}

func openWisp(lifetime, ctx context.Context, p *api.Project, stderr ...io.Writer) (*wispClient, error) {
	c, err := dockerx.Inspect(ctx, p.ContainerId)
	if err != nil {
		return nil, err
	}
	if !c.State.Running || c.Config.Labels["cxz.project"] != p.Id || c.Config.Labels["cxz.owner"] == "" {
		return nil, fmt.Errorf("refusing wisp in unowned or stopped container")
	}
	procCtx, stop := context.WithCancel(lifetime)
	cmd := exec.CommandContext(procCtx, "docker", "exec", "-i", "--user", p.RemoteUser, c.ID, "/cxz/tools/cxz", "wisp")
	if len(stderr) > 0 {
		cmd.Stderr = stderr[0]
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		stop()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		stop()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		stop()
		return nil, err
	}
	client := &wispClient{gate: make(chan struct{}, 1), enc: json.NewEncoder(in), dec: json.NewDecoder(out), stop: stop, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		if len(stderr) > 0 {
			fmt.Fprintf(stderr[0], "%s workspace helper exited: %v\n", time.Now().Format(time.RFC3339), err)
		}
		in.Close()
		close(client.done)
	}()
	ready := make(chan error, 1)
	go func() {
		var hello wisp.Response
		err := client.dec.Decode(&hello)
		client.secrets = hello.Secrets
		client.secretPolicy = hello.SecretPolicy
		if err == nil && hello.Version != wisp.Version {
			err = fmt.Errorf("incompatible wisp protocol")
		}
		ready <- err
	}()
	select {
	case <-ctx.Done():
		stop()
		return nil, ctx.Err()
	case err := <-ready:
		if err != nil {
			stop()
			return nil, fmt.Errorf("wisp unavailable; update project runtime: %w", err)
		}
		return client, nil
	}
}

// Logs observes helpers started by this TUI; stdio protocol bodies are never logged.
func (p *WispPool) Logs(project string) string {
	p.mu.Lock()
	log := p.diagnostics[project]
	p.mu.Unlock()
	if log == nil {
		return "No Wisp diagnostics collected by this TUI for this project. Earlier/other TUI connections have no shared Wisp log."
	}
	return log.String()
}
