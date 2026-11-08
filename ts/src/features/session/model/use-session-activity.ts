import { useEffect, useMemo, useState } from "react";
import type { Session } from "#gen/cxz/session_pb";
import { ref, type Connection } from "#src/shared/api/connection.ts";
import { errorNotificationOptions } from "#src/shared/errors/error-notification.ts";
import {
  activityVersion,
  loadSessionActivity,
  type ActivityCache,
} from "./session-activity-time";

const connections = new WeakMap<Connection, ActivityCache>();
export function useSessionActivity(c: Connection, sessions: Session[]) {
  const cache = useMemo(() => {
    let cache = connections.get(c);
    if (!cache) connections.set(c, (cache = new Map()));
    return cache;
  }, [c]);
  const [state, setState] = useState(() => ({ c, times: new Map(cache) }));
  const version = sessions
    .map((s) => `${s.runtimeId}/${activityVersion(s)}`)
    .join(";");
  useEffect(() => {
    const controller = new AbortController();
    const publish = () => setState({ c, times: new Map(cache) });
    publish();
    void loadSessionActivity(
      sessions,
      cache,
      controller.signal,
      (session, signal) =>
        c.sessions.transcript(
          { ref: ref(session.runtimeId), limit: 1 },
          // This timestamp is best effort; retain diagnostics without alerting
          // whenever the project inventory changes or the list is revisited.
          { signal, timeoutMs: 10_000, ...errorNotificationOptions(false) },
        ),
      publish,
    );
    return () => controller.abort();
  }, [c, cache, version]);
  return state.c === c ? state.times : cache;
}
