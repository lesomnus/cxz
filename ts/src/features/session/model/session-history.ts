import { Code, ConnectError } from "@connectrpc/connect";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import type { SessionEvent } from "#gen/cxz/session_pb";

export type HistoryPage = {
  events: SessionEvent[];
  metadata: SessionEvent[];
  snapshotSeq: bigint;
  hasOlder: boolean;
  hasNewer: boolean;
  precedingInput?: SessionEvent;
};
export type LoadEventDetails = (
  seq: bigint,
  signal: AbortSignal,
) => Promise<SessionEvent[]>;
export const HISTORY_PAGE_SIZE = 1024;
export const NATIVE_HISTORY_PAGE_SIZE = 1024;
export const HISTORY_PREFETCH_SCREENS = 6;
/**
 * How long a row stream may say nothing before it is reported as stalled. The
 * runtime heartbeats an idle stream every 10s, so this is two missed beats.
 */
export const ROW_STALL_MS = 30_000;

/**
 * Reading a conversation is reading rows, and rows come from the session's
 * runtime: Transcript pages them and Rows streams them.
 *
 * Neither has a fallback. There used to be one -- a runtime without Transcript
 * was paged natively and the records were reduced into rows here -- and that
 * fallback is why this client carried a second copy of the server's projection,
 * in a path that only ran against runtimes nobody was testing. It is also how
 * the two clients drifted: see cxz#155.
 *
 * The cost is that a project runtime older than the Manager cannot show its
 * conversation until it is updated. It says so rather than rendering something
 * reduced by a different set of rules.
 */
export class SessionHistory {
  constructor(
    private sessions: Connection["sessions"],
    private id: string,
    private clientId: string,
  ) {}
  nativePage(afterSeq: bigint, signal: AbortSignal) {
    return this.sessions.history(
      { ref: ref(this.id), afterSeq, limit: NATIVE_HISTORY_PAGE_SIZE },
      { signal },
    );
  }
  async page(
    query: { beforeSeq?: bigint; afterSeq?: bigint; snapshotSeq?: bigint },
    signal: AbortSignal,
  ): Promise<HistoryPage> {
    try {
      return await this.sessions.transcript(
        { ref: ref(this.id), ...query, limit: HISTORY_PAGE_SIZE },
        { signal },
      );
    } catch (error) {
      throw outdated(error);
    }
  }
  /**
   * snapshot is the journal's own fence, asked for as cheaply as the projection
   * can answer it: one row, for its snapshotSeq. A catalog's LastSeq can trail
   * the journal, so only this proves a conversation was reset.
   */
  async snapshot(signal: AbortSignal) {
    try {
      const page = await this.sessions.transcript(
        { ref: ref(this.id), limit: 1 },
        { signal },
      );
      return page.snapshotSeq;
    } catch (error) {
      throw outdated(error);
    }
  }
  /**
   * rows is the same projection arriving. afterUpdated is a cursor over row
   * versions rather than over records: a response row is published again when
   * its turn completes, so the same seq arrives twice and the second one is the
   * correction. Callers replace by seq.
   *
   * A row with no event is the runtime's heartbeat, not a row. It is what makes
   * silence legible: without it an open stream that has stalled looks exactly
   * like one with nothing to send, and the conversation stops growing while the
   * reader still believes it is live (cxz#188). Seeing one arms a deadline, and
   * missing the next two throws -- which is what the caller already handles,
   * because it reconnects on a stream that fails.
   */
  async *rows(afterUpdated: bigint, signal: AbortSignal) {
    const stream = this.sessions.rows(
      { ref: ref(this.id), afterUpdated, clientId: this.clientId },
      { signal },
    )[Symbol.asyncIterator]();
    // Armed by the first heartbeat rather than on open, because a runtime that
    // predates them is legitimately silent: timing it out would reconnect a
    // healthy conversation every ROW_STALL_MS forever.
    let armed = false;
    try {
      for (;;) {
        const next = await (armed
          ? deadline(stream.next(), ROW_STALL_MS)
          : stream.next());
        if (next.done) return;
        if (!next.value.event) {
          armed = true;
          continue;
        }
        yield { event: next.value.event, updated: next.value.updated };
      }
    } catch (error) {
      throw outdated(error);
    } finally {
      // Best effort, and deliberately not awaited: an iterator suspended on a
      // read that never completes -- exactly the case being reported -- would
      // never process the return, and awaiting it here would swallow the very
      // error that is being thrown. The request itself is released when the
      // next connect aborts the signal.
      void stream.return?.().catch(() => {});
    }
  }
  readonly details: LoadEventDetails = async (seq, signal) => {
    const page = await this.sessions.eventDetails(
      { ref: ref(this.id), seq },
      { signal },
    );
    return page.events;
  };
}

// deadline reports a frame that did not arrive. The abandoned read stays
// attached to the race, so its own later rejection is already handled; nothing
// is lost by dropping its value, because the only reason to be here is that
// there was no value.
function deadline<T>(read: Promise<T>, ms: number): Promise<T> {
  let timer: ReturnType<typeof setTimeout>;
  return Promise.race([
    read,
    new Promise<never>((_, reject) => {
      timer = setTimeout(
        () =>
          reject(
            new ConnectError(
              "The conversation stream stopped responding.",
              Code.Unavailable,
            ),
          ),
        ms,
      );
    }),
  ]).finally(() => clearTimeout(timer));
}

// Unimplemented from a session's runtime means that runtime predates the
// reading projection. Say which thing is behind, because the Manager being
// current is what makes this confusing: a project container keeps running its
// old binary until it is recreated.
function outdated(error: unknown) {
  if (ConnectError.from(error).code !== Code.Unimplemented) return error;
  return new ConnectError(
    "This project's runtime is older than the manager and cannot serve a conversation. Update the project runtime.",
    Code.Unimplemented,
  );
}
