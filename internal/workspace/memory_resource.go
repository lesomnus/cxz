package workspace

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/memorywire"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The same API runs beside the project's resources.db, including when stopped.
func (m *Manager) MemoryInvoke(ctx context.Context, id, method string, in, out proto.Message) error {
	p, err := m.resolve(ctx, id)
	if err != nil {
		return err
	}
	// A live project avoids container startup; both paths write the same DB.
	if conn, e := m.ClientForProject(ctx, id); e == nil {
		defer conn.Close()
		err := conn.Invoke(ctx, method, in, out)
		if status.Code(err) != codes.Unimplemented {
			return err
		}
	}
	data, err := protojson.Marshal(in)
	if err != nil {
		return err
	}
	result, err := m.memoryHelper(ctx, p.ID, "_memory-resource", memorywire.Request{Project: &api.Project{Id: p.ID, Workspace: p.Workspace, Name: p.Name, Config: p.Config, State: "stopped"}, Method: method, Data: data}, []memoryMount{m.stateMount(p, "/cxz/state", true)})
	if err != nil {
		return err
	}
	var reply memorywire.Reply
	if err = json.Unmarshal(result, &reply); err != nil {
		return err
	}
	if reply.Code != 0 {
		return status.Error(codes.Code(reply.Code), reply.Error)
	}
	return protojson.Unmarshal(reply.Data, out)
}
