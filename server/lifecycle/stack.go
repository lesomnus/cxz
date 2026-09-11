// Package lifecycle is the application's payday server layer. Generated Sink
// owns resource storage; runtime owns process/volume effects and durable journals.
package lifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/ent"
	"github.com/lesomnus/cxz/internal/ent/migrate"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/bare"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/config"
	"github.com/lesomnus/payday/watch"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"sync/atomic"
)

type Runtime interface {
	api.SessionsServer
	RegisterProject(context.Context, string, string) (*api.Project, error)
	ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error)
}
type shared struct {
	mu         sync.Mutex
	snapshotMu sync.Mutex
	watchers   atomic.Int64
	runtime    Runtime
}

// Reconcile advances payday Watch while resource subscribers exist. Journal
// Events has its own durable cursor and does not depend on this poll.
func (s Layer) Reconcile(ctx context.Context) error {
	if s.shared.watchers.Load() == 0 {
		return nil
	}
	return s.sync(ctx)
}

type Layer struct {
	resource.Overlay
	shared *shared
	bound  bool
}
type builder struct{ shared *shared }

func (b builder) Build(next resource.Server) (resource.Server, error) {
	return Layer{Overlay: resource.NewOverlay(next), shared: b.shared}, nil
}

// A separate database keeps the rc.1 registry/journal untouched. Resource
// projections can be rebuilt from runtime manifests, without changing vendor IDs.
func Build(ctx context.Context, db *sql.DB, r Runtime) (resource.Server, error) {
	driver := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(driver))
	if err := migrate.NewSchema(driver).Create(ctx); err != nil {
		return nil, err
	}
	broker, err := (config.WatchConfig{Broker: config.BrokerMemory}).Build(config.DbConfig{Driver: "sqlite3"})
	if err != nil {
		return nil, err
	}
	w := watch.New(broker)
	sink, err := pd.NewSink(client, bare.WithMinter(pd.Minter()), bare.WithScope(pd.Wall()), bare.WithRecorder(bare.Recorders{pd.Recorder(), pd.WatchRecorder(w)}))
	if err != nil {
		return nil, err
	}
	// Publish each committed Sink call, including background projection writes.
	return resource.Build(sink.WithWatch(w), pd.InterceptBuild([]grpc.UnaryServerInterceptor{w.Unary()}, nil), builder{&shared{runtime: r}}, pd.AuditBuild(), pd.GateBuild())
}
func (s Layer) WithDriver(d dialect.Driver) (resource.Server, error) {
	next, err := enttx.Rebind(s.Next(), d)
	if err != nil {
		return nil, err
	}
	return Layer{Overlay: resource.NewOverlay(next), shared: s.shared, bound: true}, nil
}

var _ enttx.Binder[resource.Server] = Layer{}

func (s Layer) effect() error {
	if s.bound {
		return status.Error(codes.FailedPrecondition, "runtime effects cannot run inside a database transaction")
	}
	return nil
}
func (s Layer) Project() resource.ProjectServiceServer { return ProjectServer{s, s.Next().Project()} }
func (s Layer) Session() resource.SessionServiceServer { return SessionServer{s, s.Next().Session()} }

// Authentication/batch administration is intentionally not exposed through the
// local operator API. No dummy tenant or fabricated user is provisioned.
func (s Layer) Tenant() resource.TenantServiceServer {
	return resource.UnimplementedTenantServiceServer{}
}
func (s Layer) Holder() resource.HolderServiceServer {
	return resource.UnimplementedHolderServiceServer{}
}
func (s Layer) Outbox() resource.OutboxServiceServer {
	return resource.UnimplementedOutboxServiceServer{}
}
func (s Layer) Audit() resource.AuditServiceServer { return resource.UnimplementedAuditServiceServer{} }

func ptr[T any](v T) *T { return &v }
func resourceID(domain byte, runtimeID string) []byte {
	h := sha256.Sum256([]byte("cxz.resource:" + runtimeID))
	id := append([]byte(nil), h[:16]...)
	id[6] = (id[6] & 15) | 0x80
	id[8] = (id[8] & 63) | 0x80
	id[9] = domain
	return id
}
func projectRef(id string) *resource.ProjectRef {
	return resource.ProjectRef_builder{RuntimeId: &id}.Build()
}
func sessionRef(id string) *resource.SessionRef {
	return resource.SessionRef_builder{RuntimeId: &id}.Build()
}
func closed() error {
	return status.Error(codes.PermissionDenied, "general resource writes are closed; use explicit lifecycle RPCs")
}
