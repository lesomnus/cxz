import { useState } from "react";
import { Button, ConfirmationDialog } from "@lesomnus/cxz-ui";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { Connection, ref } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { useResourceInventory } from "#src/shared/api/resource-inventory.ts";
import { SessionCreate } from "./session-create";
import type { Project } from "#gen/cxz/project_pb";
import type { Session } from "#gen/cxz/session_pb";
import styles from "#src/shared/components/resource-surface.module.css";

export function SessionLibrary({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  const [archived, setArchived] = useState(false);
  const [create, setCreate] = useState(false);
  const [confirm, setConfirm] = useState<Session>();
  const [error, setError] = useState("");
  const inventory = useResourceInventory(c);
  const rows = useRemote(
    async (signal) => {
      if (!archived) return [];
      const items: Session[] = [];
      let after = "";
      do {
        const page = await c.sessions.list(
          { filters: [{ listed: !archived }], after, size: 200 },
          { signal, timeoutMs: 10000 },
        );
        items.push(...page.items);
        after = page.next;
      } while (after);
      return items;
    },
    [c, archived],
  );
  return (
    <main className={styles.page}>
      <header className={styles.heading}>
        <h1>Sessions</h1>
        <Button onClick={() => setCreate(!create)}>New session</Button>
      </header>
      <div className={styles.body}>
        <div className={styles.content}>
          {create && <SessionCreate c={c} projects={projects} />}
          <div className={styles.tabs}>
            <Button aria-pressed={!archived} onClick={() => setArchived(false)}>
              Active
            </Button>
            <Button aria-pressed={archived} onClick={() => setArchived(true)}>
              Archived
            </Button>
            <Button onClick={archived ? rows.reload : c.inventory.refresh}>
              Refresh
            </Button>
          </div>
          {(rows.error || error) && <p role="alert">{rows.error || error}</p>}
          {(archived ? rows.loading : inventory.loading.sessions) && (
            <p className={styles.muted}>Loading sessions…</p>
          )}
          {(archived ? rows.data : inventory.sessions)?.map((session) => (
            <div className={styles.row} key={session.runtimeId}>
              {archived ? (
                <div className={styles.item}>
                  <span>
                    {session.name || session.alias}
                    <small>
                      {session.alias} · {session.project?.name}
                    </small>
                  </span>
                </div>
              ) : (
                <RouteLink
                  className={styles.item}
                  to="/sessions/$sessionId"
                  params={{ sessionId: session.runtimeId }}
                >
                  <span>
                    {session.name || session.alias}
                    <small>
                      {session.alias} · {session.project?.name} ·{" "}
                      {session.status?.state}
                    </small>
                  </span>
                </RouteLink>
              )}
              <Button
                onClick={async () => {
                  if (!archived) {
                    setConfirm(session);
                    return;
                  }
                  try {
                    const updated = await c.sessions.restore(
                      {
                        key: { case: "runtimeId", value: session.runtimeId },
                      },
                      { timeoutMs: 30000 },
                    );
                    c.store.apply("cxz.Session", [
                      { id: updated.id, value: updated },
                    ]);
                    rows.reload();
                  } catch (error) {
                    setError(String(error));
                  }
                }}
              >
                {archived ? "Restore" : "Archive"}
              </Button>
            </div>
          ))}
          {!(archived ? rows.loading : inventory.loading.sessions) &&
            !(archived ? rows.data : inventory.sessions)?.length && (
              <p className={styles.muted}>No sessions.</p>
            )}
          {confirm && (
            <ConfirmationDialog
              title="Archive session"
              confirmLabel="Archive"
              close={() => setConfirm(undefined)}
              execute={async () => {
                try {
                  await c.sessions.erase(ref(confirm.runtimeId), {
                    timeoutMs: 30000,
                  });
                  c.store.apply("cxz.Session", [{ id: confirm.id }]);
                  rows.reload();
                  return true;
                } catch (error) {
                  setError(String(error));
                  return false;
                }
              }}
            >
              <p>
                Stops the agent and hides this session. Its conversation and
                memory are retained, and you can restore it later.
              </p>
            </ConfirmationDialog>
          )}
        </div>
      </div>
    </main>
  );
}
