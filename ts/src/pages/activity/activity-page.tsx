import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useState } from "react";

import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import { BackgroundReport } from "#src/features/activity/background-report.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
export function ActivityPage({
  c,
  sessions,
}: {
  c: Connection;
  sessions: Session[];
}) {
  const [id, setId] = useState("");
  const session = sessions.find((s) => s.runtimeId === id);
  return (
    <main className={styles.page}>
      <header className={styles.heading}>
        <h1>Activity & usage</h1>
        <ValueMenu
          label="Scope"
          value={id}
          options={[
            { value: "", label: "All sessions" },
            ...sessions.map((s) => ({
              value: s.runtimeId,
              label: s.name || s.alias || s.runtimeId,
            })),
          ]}
          choose={setId}
        />
      </header>
      <div className={styles.body}>
        <div className={styles.content}>
          <UsageReport c={c} session={id} />
          {session ? (
            <BackgroundReport
              c={c}
              session={session.id}
              run={session.status?.runId ?? ""}
            />
          ) : (
            <section className={styles.section}>
              <strong>Current work</strong>
              {sessions
                .filter((s) =>
                  ["working", "waiting_input", "starting"].includes(
                    s.status?.state ?? "",
                  ),
                )
                .map((s) => (
                  <RouteLink
                    className={styles.item}
                    key={s.runtimeId}
                    {...sessionLocation(s)}
                  >
                    <span>
                      {s.name || s.alias}
                      <small>
                        {s.alias} · {s.status?.state}
                        {s.status?.pending.length
                          ? ` · ${s.status.pending.length} pending requests`
                          : ""}
                        {s.status?.queued ? " · Message queued" : ""}
                      </small>
                    </span>
                  </RouteLink>
                ))}
            </section>
          )}
        </div>
      </div>
    </main>
  );
}
