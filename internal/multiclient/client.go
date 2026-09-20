// Package multiclient combines independent daemon snapshots without sending
// client-only connection prefixes over the wire.
package multiclient

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Source struct {
	Name     string
	Remote   bool
	Client   api.SessionsClient
	Open     func() (api.SessionsClient, io.Closer, error)
	wake     chan struct{}
	err      error
	loaded   bool
	sessions []*api.Session
	projects []*api.Project
}
type Client struct {
	mu      sync.Mutex
	sources map[string]*Source
	names   []string
	primary string
	changed chan struct{}
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

var _ api.SessionsClient = (*Client)(nil)

func New(ctx context.Context, sources []Source, primary string) *Client {
	ctx, cancel := context.WithCancel(ctx)
	c := &Client{sources: map[string]*Source{}, primary: primary, changed: make(chan struct{}), cancel: cancel}
	for _, s := range sources {
		s := s
		s.wake = make(chan struct{}, 1)
		c.sources[s.Name] = &s
		c.names = append(c.names, s.Name)
	}
	sort.Strings(c.names)
	for _, name := range c.names {
		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.run(ctx, c.sources[name]) }()
	}
	return c
}
func (c *Client) Close() { c.cancel(); c.wg.Wait() }
func Scope(name, id string) string {
	if id == "" {
		return ""
	}
	return name + "::" + id
}
func Split(id string) (string, string) {
	a, b, ok := strings.Cut(id, "::")
	if !ok || strings.ContainsAny(a, "/\\") {
		return "", id
	}
	return a, b
}
func (c *Client) DefaultConnection() string { return c.primary }
func (c *Client) ConnectionName(ref string) string {
	name, _ := Split(ref)
	if name == "" {
		name = c.primary
	}
	return name
}

type routeKey struct{}

func (c *Client) ContextFor(ctx context.Context, ref string) context.Context {
	name := c.ConnectionName(ref)
	ctx = context.WithValue(ctx, routeKey{}, name)
	if s := c.sources[name]; s != nil && !s.Remote {
		return transport.WithLocal(ctx)
	}
	return transport.WithRemote(ctx)
}
func (c *Client) routeName(ctx context.Context, ref string) (string, string) {
	name, id := Split(ref)
	if name == "" {
		name, _ = ctx.Value(routeKey{}).(string)
		if name == "" {
			name = c.primary
		}
	}
	return name, id
}
func (c *Client) route(ctx context.Context, ref string) (string, string, api.SessionsClient, error) {
	name, id := c.routeName(ctx, ref)
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sources[name]
	if s == nil {
		return name, id, nil, fmt.Errorf("unknown connection %q", name)
	}
	if s.Client == nil {
		return name, id, nil, status.Errorf(codes.Unavailable, "connection %s unavailable", name)
	}
	return name, id, s.Client, nil
}
func (c *Client) AccountClient(ref string) resource.AccountServiceClient {
	_, _, client, err := c.route(context.Background(), ref)
	if err != nil {
		return nil
	}
	if r, ok := client.(*resourceclient.Client); ok {
		return r.Accounts
	}
	return nil
}
func (c *Client) LocalProject(p *api.Project) *api.Project {
	if p == nil {
		return nil
	}
	out := proto.Clone(p).(*api.Project)
	_, out.Id = Split(out.Id)
	return out
}
func (c *Client) ConnectionStatus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var parts []string
	for _, name := range c.names {
		s := c.sources[name]
		state := "online"
		if !s.loaded {
			state = "connecting"
		}
		if s.err != nil {
			state = "unavailable"
		}
		parts = append(parts, name+": "+state)
	}
	return strings.Join(parts, " · ")
}
func (c *Client) notifyLocked() { close(c.changed); c.changed = make(chan struct{}) }
func (c *Client) WatchChanges(ctx context.Context, _, _ []string, changed func()) error {
	for {
		c.mu.Lock()
		wake := c.changed
		c.mu.Unlock()
		changed()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		}
	}
}
func (*Client) WatchEmpty() bool { return true }

func decorateSession(name string, s *api.Session) *api.Session {
	if s == nil {
		return nil
	}
	v := proto.Clone(s).(*api.Session)
	v.Id = Scope(name, v.Id)
	v.ProjectId = Scope(name, v.ProjectId)
	v.ProjectName = projectName(v.ProjectName, v.Workspace) + " via " + name
	for _, e := range v.Pending {
		e.SessionId = Scope(name, e.SessionId)
	}
	return v
}
func projectName(name, workspace string) string {
	if name != "" {
		return name
	}
	if workspace != "" {
		return workspace
	}
	return "Project"
}
func decorateProject(name string, p *api.Project) *api.Project {
	v := proto.Clone(p).(*api.Project)
	v.Id = Scope(name, v.Id)
	v.Name = projectName(v.Name, v.Workspace) + " via " + name
	return v
}
func (c *Client) List(context.Context, *api.Empty, ...grpc.CallOption) (*api.SessionList, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := &api.SessionList{}
	for _, name := range c.names {
		for _, s := range c.sources[name].sessions {
			out.Sessions = append(out.Sessions, decorateSession(name, s))
		}
	}
	return out, nil
}
func (c *Client) Projects(context.Context, *api.Empty, ...grpc.CallOption) (*api.ProjectList, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := &api.ProjectList{}
	for _, name := range c.names {
		s := c.sources[name]
		for _, p := range s.projects {
			v := decorateProject(name, p)
			if s.err != nil {
				v.Name += " · offline"
			}
			out.Projects = append(out.Projects, v)
		}
		if len(s.projects) == 0 {
			label := "No projects"
			if !s.loaded {
				label = "Connecting"
			}
			if s.err != nil {
				label = "Unavailable"
			}
			out.Projects = append(out.Projects, &api.Project{Id: Scope(name, "@connection"), Name: label + " via " + name, State: "connection"})
		}
	}
	return out, nil
}
func (c *Client) RegisteredProjects(ctx context.Context) (*api.ProjectList, error) {
	return c.Projects(ctx, &api.Empty{})
}

func (c *Client) run(ctx context.Context, s *Source) {
	var closer io.Closer
	defer func() {
		if closer != nil {
			closer.Close()
		}
	}()
	wake := s.wake
	trigger := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	var watchCancel context.CancelFunc
	var watchers sync.WaitGroup
	defer func() {
		if watchCancel != nil {
			watchCancel()
		}
		watchers.Wait()
	}()
	signature := ""
	for {
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		client := s.Client
		c.mu.Unlock()
		var err error
		if client == nil && s.Open != nil {
			client, closer, err = s.Open()
			if err == nil {
				c.mu.Lock()
				s.Client = client
				c.mu.Unlock()
			}
		}
		var sessions []*api.Session
		var projects []*api.Project
		if client != nil {
			q, cancel := context.WithTimeout(ctx, 5*time.Second)
			var list *api.SessionList
			list, err = client.List(q, &api.Empty{})
			if err == nil {
				sessions = list.Sessions
				var p *api.ProjectList
				if r, ok := client.(interface {
					RegisteredProjects(context.Context) (*api.ProjectList, error)
				}); ok {
					p, err = r.RegisteredProjects(q)
				} else {
					p, err = client.Projects(q, &api.Empty{})
				}
				if err == nil {
					projects = p.Projects
				}
			}
			cancel()
		} else if err == nil {
			err = fmt.Errorf("connection unavailable")
		}
		c.mu.Lock()
		s.err = err
		s.loaded = true
		if err == nil {
			s.sessions = sessions
			s.projects = projects
		}
		c.notifyLocked()
		c.mu.Unlock()
		if err == nil {
			var ps, ss []string
			for _, p := range projects {
				ps = append(ps, p.Id)
			}
			for _, v := range sessions {
				ss = append(ss, v.Id)
			}
			sort.Strings(ps)
			sort.Strings(ss)
			next := strings.Join(ps, ",") + "/" + strings.Join(ss, ",")
			if next != signature {
				if watchCancel != nil {
					watchCancel()
				}
				signature = next
				if source, ok := client.(interface {
					WatchChanges(context.Context, []string, []string, func()) error
				}); ok && len(ps)+len(ss) > 0 {
					watchCtx, cancelWatch := context.WithCancel(ctx)
					watchCancel = cancelWatch
					watchers.Add(1)
					go func() {
						defer watchers.Done()
						defer cancelWatch()
						for watchCtx.Err() == nil {
							source.WatchChanges(watchCtx, ps, ss, trigger)
							trigger()
							select {
							case <-watchCtx.Done():
								return
							case <-time.After(5 * time.Second):
							}
						}
					}()
				}
			}
		}
		delay := 30 * time.Second
		if err != nil {
			delay = 3 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
			select {
			case <-ctx.Done():
				return
			case <-time.After(150 * time.Millisecond):
			}
		case <-time.After(delay):
		}
	}
}

type eventStream struct {
	grpc.ServerStreamingClient[api.Event]
	name string
}

func (s eventStream) Recv() (*api.Event, error) {
	e, err := s.ServerStreamingClient.Recv()
	if e != nil {
		e = proto.Clone(e).(*api.Event)
		e.SessionId = Scope(s.name, e.SessionId)
	}
	return e, err
}

// Only initial attach waits for the asynchronous connector. UI inspection and
// account lookup stay nonblocking while a source is connecting.
func (c *Client) waitReady(ctx context.Context, ref string) error {
	name, _ := c.routeName(ctx, ref)
	for {
		c.mu.Lock()
		s := c.sources[name]
		wake := c.changed
		done := s == nil || s.Client != nil || s.loaded
		c.mu.Unlock()
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		}
	}
}
func (c *Client) ConnectionError(ref string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sources[c.ConnectionName(ref)]
	if s != nil && s.err != nil {
		return c.ConnectionName(ref) + ": " + s.err.Error()
	}
	return ""
}
