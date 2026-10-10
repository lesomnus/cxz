package websandbox

import (
	"context"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func files(project string) map[string]string {
	return map[string]string{
		"/workspace/README.md":                       "# " + project + "\n\nThis workspace is simulated by the WASM service.\nConnect opens a simulated workspace; no Linux, Docker or real files are used.\n",
		"/workspace/src/main.go":                     "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello from " + project + "\")\n}\n",
		"/workspace/src/app.ts":                      "export const workspace = \"" + project + "\";\n\nexport function greet(name: string) {\n  return `Hello, ${name}`;\n}\n",
		"/workspace/.devcontainer/devcontainer.json": "{\n  \"name\": \"Sandbox workspace\",\n  \"image\": \"mcr.microsoft.com/devcontainers/base:debian\",\n  \"workspaceFolder\": \"/workspace\"\n}\n",
	}
}

func (p *Projects) Paths(r *resource.ProjectPathsRequest, stream grpc.ServerStreamingServer[resource.ProjectPathsReply]) error {
	project, err := p.Get(stream.Context(), resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
	if err != nil {
		return err
	}
	root := path.Clean(r.GetPath())
	if root != "/workspace" && !strings.HasPrefix(root, "/workspace/") {
		return status.Error(codes.PermissionDenied, "path outside sandbox workspace")
	}
	entries := map[string]bool{}
	found := false
	for name := range files(project.GetRuntimeId()) {
		if strings.HasPrefix(name, root+"/") {
			found = true
			rel := strings.TrimPrefix(name, root+"/")
			first, rest, dir := strings.Cut(rel, "/")
			_ = rest
			entries[first] = dir
		}
	}
	if !found {
		return status.Error(codes.NotFound, "sandbox directory not found")
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	reply := resource.ProjectPathsReply_builder{}.Build()
	for _, name := range names {
		directory := entries[name]
		reply.SetEntries(append(reply.GetEntries(), resource.ProjectPathEntry_builder{Name: &name, Directory: &directory}.Build()))
	}
	return stream.Send(reply)
}

func (p *Projects) Download(r *resource.ProjectDownloadRequest, stream grpc.ServerStreamingServer[resource.ProjectDownloadReply]) error {
	project, err := p.Get(stream.Context(), resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
	if err != nil {
		return err
	}
	text, ok := files(project.GetRuntimeId())[path.Clean(r.GetPath())]
	if !ok {
		p.S.mu.Lock()
		upload, found := p.S.uploads[r.GetPath()]
		p.S.mu.Unlock()
		if !found || upload.project != project.GetRuntimeId() {
			return status.Error(codes.NotFound, "sandbox file not found")
		}
		if len(upload.content) == 0 {
			size := int64(0)
			return stream.Send(resource.ProjectDownloadReply_builder{TotalSize: &size}.Build())
		}
		for offset := 0; offset < len(upload.content); offset += 256 * 1024 {
			size := int64(len(upload.content))
			if err := stream.Send(resource.ProjectDownloadReply_builder{Data: upload.content[offset:min(offset+256*1024, len(upload.content))], TotalSize: &size}.Build()); err != nil {
				return err
			}
		}
		return nil
	}
	size := int64(len(text))
	return stream.Send(resource.ProjectDownloadReply_builder{Data: []byte(text), TotalSize: &size}.Build())
}

func (p *Projects) Editor(ctx context.Context, r *resource.ProjectEditorRequest) (*resource.ProjectEditorReply, error) {
	project, err := p.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.S.delay):
	}
	workspace := project.GetStatus().GetRemoteWorkspace()
	simulated := true
	return resource.ProjectEditorReply_builder{Workspace: &workspace, Simulated: &simulated}.Build(), nil
}
