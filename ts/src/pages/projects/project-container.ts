import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";

type Target = Pick<Connection, "projects" | "store" | "inventory">;

// The container's lifecycle, one function per RPC so that what a button says
// and what it calls cannot drift apart. Stopping keeps the container, starting
// makes the project run -- building one when there is none -- and removing
// throws it away. None of them touches the project, its sessions or their
// conversations.
// See docs/plans/project-lifecycle-api.md.
type Control = (
  request: {
    ref: { key: { case: "id"; value: Uint8Array } };
    clientId: string;
    trustConfig?: boolean;
  },
  options: { timeoutMs: number },
) => Promise<Project>;

async function control(
  c: Target,
  project: Project,
  call: Control,
  extra: { trustConfig?: boolean } = {},
) {
  const updated = await call(
    {
      ref: { key: { case: "id", value: project.id } },
      clientId: crypto.randomUUID(),
      ...extra,
    },
    { timeoutMs: 120_000 },
  );
  c.store.apply("cxz.Project", [{ id: updated.id, value: updated }]);
  c.inventory.refresh();
  return updated;
}

export const stopContainer = (c: Target, project: Project) =>
  control(c, project, c.projects.stop as Control);
// `trusted` is the answer to the question a start can come back with: the
// configuration asks for something dangerous, and these are the findings a
// person was shown and said yes to. See config-trust.ts.
export const startContainer = (c: Target, project: Project, trusted = false) =>
  control(
    c,
    project,
    c.projects.start as Control,
    trusted ? { trustConfig: true } : {},
  );
export const removeContainer = (c: Target, project: Project) =>
  control(c, project, c.projects.remove as Control);
