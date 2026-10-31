package websandbox

import (
	"context"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"slices"
	"strings"
)

func (x *Sessions) Erase(_ context.Context, ref *resource.SessionRef) (*resource.SessionEraseResponse, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(ref)
	if status.Code(err) == codes.NotFound {
		return &resource.SessionEraseResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	s.cancelTurn(st)
	for path, upload := range s.uploads {
		if strings.HasPrefix(path, "/cxz/assets/"+st.value.GetRuntimeId()+"/") {
			s.uploadBytes -= int64(len(upload.content))
			delete(s.uploads, path)
		}
	}
	s.sessions = slices.DeleteFunc(s.sessions, func(candidate *session) bool { return candidate == st })
	s.rev++
	return resource.SessionEraseResponse_builder{Erased: proto.Bool(true)}.Build(), nil
}
