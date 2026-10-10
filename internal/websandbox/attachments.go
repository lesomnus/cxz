package websandbox

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The sandbox retains bytes in memory until reset; it never writes a host file.
func (x *Sessions) Upload(stream grpc.ClientStreamingServer[resource.SessionUploadRequest, resource.SessionAttachment]) error {
	header, err := stream.Recv()
	if err != nil {
		return err
	}
	name, size := header.GetName(), header.GetSize()
	if header.GetRef() == nil || header.GetRunId() == "" || name == "" || name == "." || name == ".." || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) || len(header.GetContent()) != 0 || size < 0 || size > 32<<20 {
		return status.Error(codes.InvalidArgument, "sandbox uploads require a filename and contain at most 32 MiB")
	}
	s := x.S
	s.mu.Lock()
	st, err := s.find(header.GetRef())
	if err == nil {
		err = checkRun(st, header.GetRunId())
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	var content bytes.Buffer
	for {
		if err := stream.Context().Err(); err != nil {
			return err
		}
		part, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if part.HasRef() || part.HasRunId() || part.HasName() || part.HasSize() || int64(content.Len()+len(part.GetContent())) > size {
			return status.Error(codes.InvalidArgument, "invalid upload chunk")
		}
		_, _ = content.Write(part.GetContent())
	}
	if int64(content.Len()) != size {
		return status.Error(codes.InvalidArgument, "upload size mismatch")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err = s.find(header.GetRef())
	if err == nil {
		err = checkRun(st, header.GetRunId())
	}
	if err != nil {
		return err
	}
	if s.uploadBytes+size > 128<<20 {
		return status.Error(codes.ResourceExhausted, "reset the sandbox to release uploaded files")
	}
	s.uploadSequence++
	path := fmt.Sprintf("/cxz/assets/%s/upload-%d/%s", st.value.GetRuntimeId(), s.uploadSequence, name)
	if s.uploads == nil {
		s.uploads = make(map[string]sandboxAttachment)
	}
	s.uploads[path] = sandboxAttachment{project: st.value.GetProject().GetRuntimeId(), content: content.Bytes()}
	s.uploadBytes += size
	return stream.SendAndClose(resource.SessionAttachment_builder{Path: &path}.Build())
}

type sandboxAttachment struct {
	project string
	content []byte
}
