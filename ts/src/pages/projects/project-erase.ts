import { key } from "@lesomnus/payday/store";
import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";

type EraseConnection = Pick<
  Connection,
  "projects" | "store" | "drafts" | "attention" | "conversations" | "inventory"
>;
// Erase deletes what the project owns. The ref is the UUID this page is looking
// at, so there is no string for the server to resolve and no preview needed to
// check that it resolved to the right thing.
export async function eraseProject(
  c: EraseConnection,
  target: Project,
  knownSessions: Session[],
) {
  if (!target.id.length)
    throw new Error(t("Project changed. Close this dialog and try again."));
  const result = await c.projects.erase(
    { key: { case: "id", value: target.id } },
    { timeoutMs: 120_000 },
  );
  if (!result.erased)
    throw new Error(t("Project removal was not confirmed by the server."));
  const sessions = new Map(
    [...knownSessions, ...c.inventory.snapshot().sessions]
      .filter((s) => s.project && key(s.project.id) === key(target.id))
      .map((s) => [s.runtimeId, s]),
  );
  c.store.apply("cxz.Project", [{ id: target.id }]);
  c.store.apply(
    "cxz.Session",
    [...sessions.values()].map((s) => ({ id: s.id })),
  );
  for (const s of sessions.values()) {
    c.drafts.delete(s.runtimeId);
    c.attention.reset(s.runtimeId);
    c.conversations.delete(s.runtimeId);
  }
  c.inventory.refresh();
}
