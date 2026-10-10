package websandbox

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
)

// Purge is an in-memory fixture: no filesystem, runtime or account is touched.
func (x *Sessions) Purge(_ context.Context, r *resource.SessionPurgeRequest) (*resource.SessionPurgeReply, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	journal, _ := json.Marshal(st.events)
	result := resource.SessionPurgeReply_builder{
		Ref: proto.Clone(r.GetRef()).(*resource.SessionRef), DryRun: proto.Bool(r.GetDryRun()),
		Targets: []*resource.SessionPurgeTarget{
			resource.SessionPurgeTarget_builder{Kind: proto.String("journal"), Path: proto.String("/sandbox/" + st.value.GetRuntimeId() + "/journal"), Files: proto.Int32(1), Bytes: proto.Int64(int64(len(journal)))}.Build(),
			resource.SessionPurgeTarget_builder{Kind: proto.String("record"), Path: proto.String("sandbox session catalog"), Files: proto.Int32(1)}.Build(),
		},
		Retained: []string{"Project workspace", "Shared account settings"},
	}.Build()
	if !r.GetDryRun() {
		s.cancelTurn(st)
		for path, upload := range s.uploads {
			if strings.HasPrefix(path, "/cxz/assets/"+st.value.GetRuntimeId()+"/") {
				s.uploadBytes -= int64(len(upload.content))
				delete(s.uploads, path)
			}
		}
		s.sessions = slices.DeleteFunc(s.sessions, func(candidate *session) bool { return candidate == st })
		s.rev++
	}
	return result, nil
}
