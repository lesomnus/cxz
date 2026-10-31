import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { useRef, useState } from "react";
import { Button } from "@lesomnus/nox";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { Connection } from "#src/shared/api/connection.ts";
import { useResourceInventory } from "#src/shared/api/resource-inventory.ts";
import { SessionCreate } from "./session-create";
import type { Project } from "#gen/cxz/project_pb";
import styles from "#src/shared/components/resource-surface.module.css";

export function SessionLibrary({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  const anchor = useRef<HTMLElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const [create, setCreate] = useState(false);
  const inventory = useResourceInventory(c);
  return (
    <main className={styles.page}>
      <header ref={anchor} className={styles.heading}>
        <h1>Sessions</h1>
        <Button ref={trigger} onClick={() => setCreate(true)}>
          New session
        </Button>
      </header>
      <div className={styles.body}>
        <div className={styles.content}>
          {create && (
            <SessionCreate
              c={c}
              projects={projects}
              anchor={anchor.current}
              trigger={trigger.current}
              close={() => setCreate(false)}
            />
          )}
          <Button onClick={c.inventory.refresh}>Refresh</Button>
          {inventory.loading.sessions && (
            <p className={styles.muted}>Loading sessions…</p>
          )}
          {inventory.sessions.map((session) => (
            <RouteLink
              key={session.runtimeId}
              className={styles.item}
              {...sessionLocation(session)}
            >
              <span>
                {session.name || session.alias}
                <small>
                  {session.alias} · {session.project?.name} ·{" "}
                  {session.status?.state}
                </small>
              </span>
            </RouteLink>
          ))}
          {!inventory.loading.sessions && !inventory.sessions.length && (
            <p className={styles.muted}>No sessions.</p>
          )}
        </div>
      </div>
    </main>
  );
}
