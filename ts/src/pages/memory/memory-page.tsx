import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useState } from "react";

import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { matchesResource } from "#src/shared/navigation/resource-route.ts";
export function MemoryPage({
  c,
  sessions,
}: {
  c: Connection;
  sessions: Session[];
}) {
  const [selected, setSelected] = useState("");
  const { memoryProject } = useWorkspaceRoute();
  const scoped = sessions.filter(
    (s) =>
      !memoryProject ||
      (!!s.project && matchesResource(s.project, memoryProject)),
  );
  const session =
    scoped.find((s) => s.runtimeId === selected)?.runtimeId ||
    scoped[0]?.runtimeId ||
    "";
  return (
    <main className={styles.page}>
      <header className={styles.heading}>
        <h1>Shared memory</h1>
        <ValueMenu
          label="Session"
          value={session}
          options={scoped.map((s) => ({
            value: s.runtimeId,
            label: `${s.project?.name || s.project?.alias || ""} · ${s.alias}`,
          }))}
          choose={setSelected}
        />
      </header>
      <div className={styles.body}>
        {session ? (
          <MemoryLibrary key={session} c={c} session={session} />
        ) : (
          <p className={styles.muted}>
            Create a session to open its project's shared memory.
          </p>
        )}
      </div>
    </main>
  );
}
