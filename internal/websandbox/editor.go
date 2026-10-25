package websandbox

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"
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
		"/home/sandbox/notes.txt":                    "Sandbox home for " + project + "\n",
		"/home/sandbox/.config/settings.json":        "{\"sandbox\": true}\n",
		"/home/sandbox/demo/notes.txt":               "Sandbox path hints\n",
		"/home/sandbox/demo/run.sh":                  "#!/bin/sh\nprintf 'Sandbox script\\n'\n",
	}
}

func sandboxPath(requested string) string {
	if requested == "~" || strings.HasPrefix(requested, "~/") {
		requested = "/home/sandbox" + strings.TrimPrefix(requested, "~")
	}
	if !strings.HasPrefix(requested, "/") {
		requested = "/workspace/" + requested
	}
	return path.Clean(requested)
}

func (p *Projects) Paths(r *resource.ProjectPathsRequest, stream grpc.ServerStreamingServer[resource.ProjectPathsReply]) error {
	project, err := p.Get(stream.Context(), resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
	if err != nil {
		return err
	}
	root := sandboxPath(r.GetPath())
	if root != "/" && root != "/home" && root != "/home/sandbox" && !strings.HasPrefix(root, "/home/sandbox/") && root != "/workspace" && !strings.HasPrefix(root, "/workspace/") {
		return status.Error(codes.PermissionDenied, "path outside sandbox workspace")
	}
	entries := map[string]bool{}
	found := false
	prefix := strings.TrimSuffix(root, "/") + "/"
	for name := range files(project.GetAlias()) {
		if strings.HasPrefix(name, prefix) {
			found = true
			rel := strings.TrimPrefix(name, prefix)
			first, rest, dir := strings.Cut(rel, "/")
			_ = rest
			entries[first] = dir
		}
	}
	if !found {
		return status.Error(codes.NotFound, "sandbox directory not found")
	}
	if root == "/home/sandbox/demo" {
		for _, name := range []string{"channel.pipe", "control.sock", "notes-link"} {
			entries[name] = false
		}
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	reply := resource.ProjectPathsReply_builder{}.Build()
	for _, name := range names {
		directory := entries[name]
		kind := "file"
		if directory {
			kind = "directory"
		}
		entry := resource.ProjectPathEntry_builder{Name: &name, Directory: &directory, Kind: &kind}.Build()
		if root == "/home/sandbox/demo" {
			switch name {
			case "run.sh":
				entry.SetExecutable(true)
			case "channel.pipe":
				entry.SetKind("fifo")
			case "control.sock":
				entry.SetKind("socket")
			case "notes-link":
				entry.SetKind("symlink")
				entry.SetSymlink(true)
				entry.SetLinkTarget("notes.txt")
			}
		}
		reply.SetEntries(append(reply.GetEntries(), entry))
	}
	return stream.Send(reply)
}

func (p *Projects) Download(r *resource.ProjectDownloadRequest, stream grpc.ServerStreamingServer[resource.ProjectDownloadReply]) error {
	project, err := p.Get(stream.Context(), resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
	if err != nil {
		return err
	}
	text, ok := files(project.GetAlias())[sandboxPath(r.GetPath())]
	if !ok {
		p.S.mu.Lock()
		upload, found := p.S.uploads[r.GetPath()]
		p.S.mu.Unlock()
		if !found || upload.project != projectid.String(project.GetId()) {
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
