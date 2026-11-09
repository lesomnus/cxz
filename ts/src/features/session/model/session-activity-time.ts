import type { Session, SessionEvent } from "#gen/cxz/session_pb";

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

/** Conversation recency, independent of metadata edits or lifecycle state. */
export function sortSessionsByActivity(
  sessions: readonly Session[],
  activity: ReadonlyMap<string, SessionActivity>,
) {
  const created = (session: Session) =>
    Number(session.dateCreated?.seconds ?? 0n) * 1000 +
    (session.dateCreated?.nanos ?? 0) / 1_000_000;
  const time = (session: Session) =>
    activity.get(session.runtimeId)?.time ?? created(session);
  return [...sessions].sort(
    (a, b) =>
      time(b) - time(a) ||
      created(b) - created(a) ||
      a.runtimeId.localeCompare(b.runtimeId),
  );
}
