import { key } from "@lesomnus/payday/store";
import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import { isSessionWorking } from "#src/features/session/components/session-activity.ts";

export type SessionActivity = { version: string; time?: number };
export type ActivityCache = Map<string, SessionActivity>;
export function activityVersion(session: Session) {
  return `${session.status?.runId ?? ""}/${session.status?.lastSeq ?? 0n}/${session.status?.state ?? ""}`;
}
export function activityTime(
  events: readonly SessionEvent[],
  precedingInput?: SessionEvent,
) {
  const times = [...events, ...(precedingInput ? [precedingInput] : [])]
    .map((e) => Number(e.timeMs))
    .filter((time) => Number.isFinite(time) && time > 0);
  return times.length ? Math.max(...times) : undefined;
}
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
export async function loadSessionActivity(
  sessions: readonly Session[],
  cache: ActivityCache,
  signal: AbortSignal,
  read: (
    session: Session,
    signal: AbortSignal,
  ) => Promise<{ events: SessionEvent[]; precedingInput?: SessionEvent }>,
  changed: () => void,
) {
  const pending = sessions.filter(
    (s) => cache.get(s.runtimeId)?.version !== activityVersion(s),
  );
  let next = 0;
  async function worker() {
    while (!signal.aborted && next < pending.length) {
      const session = pending[next++]!,
        version = activityVersion(session);
      try {
        const page = await read(session, signal);
        if (signal.aborted) return;
        cache.set(session.runtimeId, {
          version,
          time: activityTime(page.events, page.precedingInput),
        });
        changed();
      } catch {
        // Keep the last successful timestamp until a later inventory update.
      }
    }
  }
  await Promise.all(
    Array.from({ length: Math.min(3, pending.length) }, worker),
  );
}
