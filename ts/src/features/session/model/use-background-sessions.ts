import { useEffect, useMemo, useSyncExternalStore } from "react";
import type { Connection } from "#src/shared/api/connection.ts";
import type { Session } from "#gen/cxz/session_pb";
import { SessionBackground } from "./session-background";

export function useBackgroundSessions(c: Connection, sessions: Session[]) {
  const tracker = useMemo(() => new SessionBackground(c.sessions), [c]);
  const active = useSyncExternalStore(tracker.subscribe, tracker.snapshot);
  useEffect(() => () => tracker.dispose(), [tracker]);
  useEffect(() => {
    tracker.observe(sessions);
  }, [tracker, sessions]);
  return active;
}
