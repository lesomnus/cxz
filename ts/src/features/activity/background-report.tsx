import { useEffect, useRef } from "react";
import { Button } from "@lesomnus/nox";
import { Connection } from "#src/shared/api/connection.ts";
import { sessionRef, sessionUUID } from "#src/shared/api/session-id.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import styles from "#src/shared/components/resource-surface.module.css";
import { backgroundTasks } from "./background-tasks";
export { backgroundTasks } from "./background-tasks";
export function BackgroundReport({
  c,
  session,
  run,
}: {
  c: Connection;
  session: Uint8Array | undefined;
  run: string;
}) {
  const uuid = sessionUUID(session);
  const snapshot = useRemote(
    async (signal) => {
      if (!uuid || !session || !run) return [];
      const reply = await c.sessions.background(
        { ref: sessionRef(session) },
        { signal, timeoutMs: 10000 },
      );
      const state = JSON.parse(new TextDecoder().decode(reply.data) || "{}")[
        run
      ];
      if (state?.Unavailable)
        throw new Error("Background job status unavailable");
      return backgroundTasks(reply.data, run);
    },
    [c, uuid, run],
  );
  const refresh = useRef(snapshot.reload);
  refresh.current = snapshot.reload;
  useEffect(() => {
    if (!uuid || !run) return;
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") refresh.current();
    }, 10000);
    return () => clearInterval(timer);
  }, [uuid, run]);
  return (
    <section className={styles.section}>
      <div className={styles.row}>
        <strong>Background tasks</strong>
        <Button onClick={snapshot.reload}>Refresh</Button>
      </div>
      {snapshot.error && <p role="alert">{snapshot.error}</p>}
      {snapshot.data?.map((task) => (
        <div key={task.task_id}>
          <strong>{task.description || task.task_id}</strong>
          <p className={styles.status}>
            {task.active
              ? "working"
              : task.status === "working"
                ? "Awaiting final status"
                : task.status}
          </p>
          {task.summary && <p>{task.summary}</p>}
          {task.output_file && (
            <small className={styles.status}>{task.output_file}</small>
          )}
        </div>
      ))}
      {!snapshot.loading && !snapshot.error && !snapshot.data?.length && (
        <p className={styles.muted}>
          No provider-reported background tasks for this run.
        </p>
      )}
    </section>
  );
}
