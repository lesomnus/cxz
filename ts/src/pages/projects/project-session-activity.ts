import { key } from "@lesomnus/payday/store";
import type { Session } from "#gen/cxz/session_pb";
import { isSessionWorking } from "#src/features/session/components/session-activity.ts";
import type { SessionActivity } from "#src/features/session/model/session-activity-time.ts";

export function projectActivitySessions(
  sessions: readonly Session[],
  completed: ReadonlySet<string>,
  activity: ReadonlyMap<string, SessionActivity>,
) {
  const chosen = new Map<string, Session>();
  const rank = (s: Session) =>
    completed.has(s.runtimeId) ? 0 : isSessionWorking(s) ? 1 : 2;
  for (const session of sessions) {
    if (!session.project || session.dateErased) continue;
    const id = key(session.project.id),
      old = chosen.get(id);
    if (
      !old ||
      rank(session) < rank(old) ||
      (rank(session) === rank(old) &&
        (activity.get(session.runtimeId)?.time ?? 0) >
          (activity.get(old.runtimeId)?.time ?? 0))
    )
      chosen.set(id, session);
  }
  return chosen;
}
