import { create } from "@bufbuild/protobuf";
import {
  SessionSchema,
  type Session,
  type SessionEvent,
} from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import {
  sessionInfo,
  type QuotaWindow,
} from "#src/features/session/model/session-info.ts";

// Sessions have independent sequence numbers. Only timestamps order snapshots
// from different sessions; a failed provider probe must not erase known usage.
export function accountQuota(
  agent: string,
  events: SessionEvent[],
  now = Date.now(),
): QuotaWindow | undefined {
  const session = create(SessionSchema, { agent });
  const snapshots = events
    .filter(
      (event) =>
        event.kind === "usage" &&
        sessionInfo(session, [event]).quotas?.some(
          (window) => window.remaining !== undefined,
        ),
    )
    .sort((a, b) => (a.timeMs < b.timeMs ? -1 : a.timeMs > b.timeMs ? 1 : 0));
  return sessionInfo(session, snapshots)
    .quotas?.filter(
      (window) =>
        window.remaining !== undefined &&
        (window.reset === undefined || window.reset > now) &&
        (agent !== "codex" || window.bucket === "codex"),
    )
    .sort((a, b) => a.remaining! - b.remaining!)[0];
}

// Read one transcript row only for its retained metadata. No provider request,
// conversation subscription, or full journal is needed for the account card.
export async function readAccountQuota(
  c: Pick<Connection, "sessions">,
  agent: string,
  sessions: Session[],
  signal: AbortSignal,
) {
  const ids = [...new Set(sessions.map((session) => session.runtimeId))].filter(
    Boolean,
  );
  const snapshots: SessionEvent[][] = Array.from(
    { length: ids.length },
    () => [],
  );
  let next = 0;
  let succeeded = 0;
  let failure: unknown;
  await Promise.all(
    Array.from({ length: Math.min(4, ids.length) }, async () => {
      while (next < ids.length) {
        signal.throwIfAborted();
        const index = next++;
        try {
          const page = await c.sessions.transcript(
            { ref: ref(ids[index]), limit: 1 },
            { signal, timeoutMs: 10000 },
          );
          snapshots[index] = page.metadata;
          succeeded++;
        } catch (error) {
          signal.throwIfAborted();
          failure = error;
        }
      }
    }),
  );
  signal.throwIfAborted();
  if (ids.length && !succeeded) throw failure;
  return accountQuota(agent, snapshots.flat());
}
