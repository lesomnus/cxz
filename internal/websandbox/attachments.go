package websandbox

import (
	"context"
	"fmt"
	"io"

	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// A sandbox file is smaller than a real one, and all of them together are
// smaller still: the bytes live in the page's memory until a reload, and a
// person trying the upload affordance is not moving a disk image through it.
const SandboxUploadMax = 32 << 20
const SandboxUploadBudget = 128 << 20

// The sandbox retains bytes in memory until reset; it never writes a host file.
//
// Filenames, declared sizes, run identifiers and the stream's own length are
// not checked here, because they are not the sandbox's to check: the same raw
// HTTP handler the Manager serves sits in front of this, so a page that uploads
// something the real server would refuse sees the real server's refusal. What
// is left is what belongs to a fake -- a memory budget rather than a filesystem
// -- and minting a path the way the real store mints one.
func (x *Sessions) UploadAttachment(ctx context.Context, in assets.Upload, body io.Reader) (string, error) {
	if in.Size > SandboxUploadMax {
		return "", status.Errorf(codes.InvalidArgument, "sandbox uploads contain at most %d MiB", SandboxUploadMax>>20)
	}
	ref := resource.SessionRef_builder{RuntimeId: proto.String(in.SessionID)}.Build()
	s := x.S
	// Refused before the body is read as well as after it, so an upload against
	// a stale run does not first spend the transfer.
	s.mu.Lock()
	st, err := s.find(ref)
	if err == nil {
		err = checkRun(st, in.RunID)
	}
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	content := make([]byte, in.Size)
	if _, err := io.ReadFull(body, content); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return "", assets.ErrSizeMismatch
		}
		return "", err
	}
	// A source with a byte left in it declared the wrong size. Storing the
	// declared prefix would publish a file nobody chose.
	var excess [1]byte
	if n, _ := body.Read(excess[:]); n > 0 {
		return "", assets.ErrSizeMismatch
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err = s.find(ref)
	if err == nil {
		err = checkRun(st, in.RunID)
	}
	if err != nil {
		return "", err
	}
	if s.uploadBytes+in.Size > SandboxUploadBudget {
		return "", status.Error(codes.ResourceExhausted, "reset the sandbox to release uploaded files")
	}
	s.uploadSequence++
	path := fmt.Sprintf("/cxz/assets/%s/upload-%d/%s", st.value.GetRuntimeId(), s.uploadSequence, in.Name)
	if s.uploads == nil {
		s.uploads = make(map[string]sandboxAttachment)
	}
	s.uploads[path] = sandboxAttachment{project: st.value.GetProject().GetRuntimeId(), content: content}
	s.uploadBytes += in.Size
	return path, nil
}

type sandboxAttachment struct {
	project string
	content []byte
}
