import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";

export async function stopProject(
  c: Pick<Connection, "projects" | "store" | "inventory">,
  project: Project,
) {
  const updated = await c.projects.down(
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
