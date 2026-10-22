import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";

type Target = Pick<Connection, "projects" | "store" | "inventory">;

// The container's lifecycle, one function per RPC so that what a button says
// and what it calls cannot drift apart. Stopping keeps the container, starting
// brings it back, removing throws it away -- and none of them touches the
// project, its sessions or their conversations.
// See docs/plans/project-lifecycle-api.md.
type Control = (
  request: { ref: { key: { case: "id"; value: Uint8Array } }; clientId: string },
  options: { timeoutMs: number },
) => Promise<Project>;

async function control(c: Target, project: Project, call: Control) {
  const updated = await call(
    {
      ref: { key: { case: "id", value: project.id } },
      clientId: crypto.randomUUID(),
    },
    { timeoutMs: 120_000 },
  );
  c.store.apply("cxz.Project", [{ id: updated.id, value: updated }]);
  c.inventory.refresh();
  return updated;
}

export const stopContainer = (c: Target, project: Project) =>
  control(c, project, c.projects.stopContainer as Control);
export const startContainer = (c: Target, project: Project) =>
  control(c, project, c.projects.startContainer as Control);
export const removeContainer = (c: Target, project: Project) =>
  control(c, project, c.projects.removeContainer as Control);
