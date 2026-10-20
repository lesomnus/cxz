import { key } from "@lesomnus/payday/store";
import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";

type PurgeConnection = Pick<
  Connection,
  "projects" | "store" | "drafts" | "attention" | "conversations" | "inventory"
>;
export async function purgeProject(
  c: PurgeConnection,
  target: Project,
  knownSessions: Session[],
) {
  // Remove resolves a string target; verify the UUID before confirming it.
  const preview = await c.projects.remove(
    { target: target.runtimeId, confirmed: false },
    { timeoutMs: 20_000 },
  );
  if (
    !target.id.length ||
    !preview.project ||
    key(preview.project.id) !== key(target.id) ||
    preview.project.runtimeId !== target.runtimeId
  )
    throw new Error(t("Project changed. Close this dialog and try again."));
  const result = await c.projects.remove(
    { target: target.runtimeId, confirmed: true },
    { timeoutMs: 120_000 },
  );
  if (!result.removed)
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
