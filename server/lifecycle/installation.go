package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

// The installation itself: its engine, its retention budgets, its agent
// profiles, the configuration it shares with every project, and what it can say
// about its own calls. None of it names a project, which is why none of it is
// on ProjectService any more -- see docs/plans/service-boundaries.md.
//
// It is not an entity, so there is no generated server under this one. A method
// left unimplemented refuses rather than reading a table that does not exist,
// which is what the embedded Unimplemented gives.
type InstallationServer struct {
	Layer
	resource.UnimplementedInstallationServiceServer
}

func (s Layer) Installation() resource.InstallationServiceServer {
	return InstallationServer{Layer: s}
}

func (s InstallationServer) ForeignContainers(ctx context.Context, _ *resource.ForeignContainersRequest) (*resource.ForeignContainersReply, error) {
	ps, err := s.shared.runtime.Projects(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	var items []*resource.ForeignContainer
	for _, p := range ps.Projects {
		if p.State == "foreign" {
			items = append(items, resource.ForeignContainer_builder{ContainerId: &p.ContainerId, Workspace: &p.Workspace, Name: &p.Name}.Build())
		}
	}
	return resource.ForeignContainersReply_builder{Items: items}.Build(), nil
}

func (s InstallationServer) SaveSharedFiles(ctx context.Context, r *resource.SaveSharedFilesRequest) (*resource.InstallationReceipt, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.FileMappings(ctx, &api.FileMappingsInput{Bundle: r.GetBundle()})
	if err != nil {
		return nil, err
	}
	return resource.InstallationReceipt_builder{Status: &out.Status}.Build(), nil
}

func (s InstallationServer) SaveDevcontainer(ctx context.Context, r *resource.SaveDevcontainerRequest) (*resource.InstallationReceipt, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Devcontainer(ctx, &api.DevcontainerInput{Spec: r.GetSpec()})
	if err != nil {
		return nil, err
	}
	return resource.InstallationReceipt_builder{Status: &out.Status}.Build(), nil
}

// RenderDevcontainer passes the handle through rather than resolving it. It
// may name a directory inside a workspace, and only the manager knows which
// registered workspace owns that directory -- a ref resolved here would have
// to be the workspace already, which is the thing the caller is asking about.
func (s InstallationServer) RenderDevcontainer(ctx context.Context, r *resource.RenderDevcontainerRequest) (*resource.RenderDevcontainerReply, error) {
	out, err := s.shared.runtime.RenderDevcontainer(ctx, &api.RenderDevcontainerInput{Handle: r.GetHandle()})
	if err != nil {
		return nil, err
	}
	reply := resource.RenderDevcontainerReply_builder{
		Project: &out.Project, Name: &out.Name, Workspace: &out.Workspace, Note: &out.Note,
	}
	for _, f := range out.Files {
		reply.Files = append(reply.Files, resource.RenderedFile_builder{
			Name: &f.Name, Source: &f.Source, Role: &f.Role, Data: f.Data,
		}.Build())
	}
	return reply.Build(), nil
}
