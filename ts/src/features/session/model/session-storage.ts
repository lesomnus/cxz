import { ref, type Connection } from "#src/shared/api/connection.ts";
import type { Session } from "#gen/cxz/session_pb";

export type SessionSize = {
  journal: bigint;
  provider: bigint;
  profile: bigint;
  total: bigint;
};

/** Binary units, because this is what a filesystem reports. */
export function formatBytes(value: bigint): string {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let size = Number(value);
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  // A size is read to compare, so one decimal is enough once past bytes, and
  // none below a kibibyte where a decimal would be noise.
  const digits = unit === 0 ? 0 : size >= 100 ? 0 : 1;
  return `${size.toFixed(digits)} ${units[unit]}`;
}

/**
 * Sizes are measured by walking files, so each one is a request. They are
 * fetched a few at a time rather than all at once: a project's list can be
 * long, and a burst of walks competes with the sessions that are running.
 */
export async function loadSessionSizes(
  client: Pick<Connection["sessions"], "storage">,
  sessions: Pick<Session, "runtimeId">[],
  options: { signal?: AbortSignal; concurrency?: number } = {},
): Promise<Map<string, SessionSize>> {
  const out = new Map<string, SessionSize>();
  const queue = sessions.map((s) => s.runtimeId).filter(Boolean);
  const workers = Math.min(options.concurrency ?? 4, queue.length);
  let next = 0;
  await Promise.all(
    Array.from({ length: workers }, async () => {
      while (next < queue.length) {
        if (options.signal?.aborted) return;
        const id = queue[next++];
        try {
          const reply = await client.storage(ref(id), {
            signal: options.signal,
          });
          out.set(id, {
            journal: reply.journalBytes,
            provider: reply.providerBytes,
            profile: reply.profileBytes,
            total: reply.totalBytes,
          });
        } catch {
          // One session that cannot be measured -- erased while the list was
          // open, or a project whose runtime is unreachable -- leaves its own
          // cell empty instead of emptying the column.
        }
      }
    }),
  );
  return out;
}
