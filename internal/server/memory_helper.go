package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memorymigration"
	"github.com/lesomnus/cxz/internal/memorywire"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/sqlitetime"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/lifecycle"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type memoryHelperRuntime struct {
	*Server
	project *api.Project
}

func (s memoryHelperRuntime) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	sessions, err := s.Server.List(ctx, &api.Empty{})
	if err != nil {
		return nil, nil, err
	}
	for _, session := range sessions.GetSessions() {
		session.ProjectId = s.project.Id
	}
	return &api.ProjectList{Projects: []*api.Project{s.project}}, sessions, nil
}

// ServeMemory executes one typed call without booting a project or binding a
// socket. The Manager has already checked the retained volume's owner/project.
func ServeMemory(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	var request memorywire.Request
	if err := json.NewDecoder(io.LimitReader(in, 4*1024*1024)).Decode(&request); err != nil {
		return err
	}
	if request.Project.GetId() == "" || !strings.HasPrefix(request.Method, "/cxz.MemoryService/") {
		return fmt.Errorf("invalid memory helper request")
	}
	runtime, err := workspace.LoadRuntime(root)
	if err != nil {
		return fmt.Errorf("read retained project identity: %w", err)
	}
	if runtime.ProjectID == "" || projectid.Canonical(runtime.ProjectID) != projectid.Canonical(request.Project.Id) {
		return fmt.Errorf("retained project identity does not match request")
	}
	if err := memorymigration.AsOwner(root); err != nil {
		return err
	}
	open := func(name string) config.DbConfig {
		return config.DbConfig{Driver: "sqlite3", Dsn: (&url.URL{Scheme: "file", Path: filepath.Join(root, name)}).String() + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", MaxOpenConns: 1}
	}
	runtimeDB, _, err := open("cxz.db").Open(ctx)
	if err != nil {
		return err
	}
	defer runtimeDB.Close()
	if _, err = runtimeDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY,manifest BLOB NOT NULL,create_id TEXT UNIQUE NOT NULL)`); err != nil {
		return err
	}
	db, _, err := open("resources.db").Open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	r := memoryHelperRuntime{Server: &Server{root: root, db: runtimeDB}, project: request.Project}
	if err = sqlitetime.Migrate(ctx, db); err != nil {
		return err
	}
	stack, err := lifecycle.Build(ctx, db, r, nil, lifecycle.Options{RetainedMemory: true})
	if err != nil {
		return err
	}
	layer, _ := resource.Find[lifecycle.Layer](stack)
	if err = layer.MigrateMemories(ctx); err != nil {
		return err
	}
	method := strings.TrimPrefix(request.Method, "/cxz.MemoryService/")
	for _, m := range resource.MemoryService_ServiceDesc.Methods {
		if m.MethodName != method {
			continue
		}
		result, e := m.Handler(stack.Memory(), ctx, func(value any) error { return protojson.Unmarshal(request.Data, value.(proto.Message)) }, nil)
		reply := memorywire.Reply{}
		if e != nil {
			st := status.Convert(e)
			reply.Code = int32(st.Code())
			reply.Error = st.Message()
		} else {
			reply.Data, e = protojson.Marshal(result.(proto.Message))
			if e != nil {
				return e
			}
		}
		return json.NewEncoder(out).Encode(reply)
	}
	return fmt.Errorf("unknown MemoryService method %s for project %s", method, projectid.Canonical(request.Project.Id))
}
