package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/internal/projectid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Legacy references are translated before authorization and storage, including
// project refs nested in filters, batches and streaming handshakes.
func projectReferences(message proto.Message) {
	var visit func(protoreflect.Message)
	visit = func(m protoreflect.Message) {
		if m.Descriptor().FullName() == "cxz.ProjectRef" {
			legacy := m.Descriptor().Fields().ByNumber(10)
			if legacy != nil && m.Has(legacy) {
				id := m.Get(legacy).String()
				m.Clear(legacy)
				if id != "" {
					m.Set(m.Descriptor().Fields().ByNumber(1), protoreflect.ValueOfBytes(projectid.Bytes(id)))
				}
			}
			return
		}
		m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			if f.IsMap() {
				if f.MapValue().Message() != nil {
					v.Map().Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool { visit(value.Message()); return true })
				}
			} else if f.Message() != nil {
				if f.IsList() {
					for i := 0; i < v.List().Len(); i++ {
						visit(v.List().Get(i).Message())
					}
				} else {
					visit(v.Message())
				}
			}
			return true
		})
	}
	if message != nil {
		visit(message.ProtoReflect())
	}
}

func projectReferencesUnary(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if m, ok := req.(proto.Message); ok {
		projectReferences(m)
	}
	return next(ctx, req)
}

type projectReferenceStream struct{ grpc.ServerStream }

func (s projectReferenceStream) RecvMsg(v any) error {
	if err := s.ServerStream.RecvMsg(v); err != nil {
		return err
	}
	if m, ok := v.(proto.Message); ok {
		projectReferences(m)
	}
	return nil
}
func projectReferencesStream(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	return next(srv, projectReferenceStream{stream})
}
