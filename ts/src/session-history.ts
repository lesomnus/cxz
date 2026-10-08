import { Code, ConnectError } from "@connectrpc/connect";
import type { Connection } from "./connection";
import { ref } from "./connection";
import type { SessionEvent } from "../gen/cxz/session_pb";
import { mergeEvents } from "./journal";

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

// Capability belongs to this session's runtime: a gateway can route to projects
// running different versions. Only Unimplemented enables the legacy fallback.
export class SessionHistory {
  mode: "unknown" | "summary" | "legacy" = "unknown";
  constructor(
    private sessions: Connection["sessions"],
    private id: string,
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
    if (this.mode !== "legacy") {
      try {
        const page = await this.sessions.transcript(
          { ref: ref(this.id), ...query, limit: HISTORY_PAGE_SIZE },
          { signal },
        );
        this.mode = "summary";
        return page;
      } catch (error) {
        if (ConnectError.from(error).code !== Code.Unimplemented) throw error;
        this.mode = "legacy";
      }
    }
    const snapshotSeq = query.snapshotSeq ?? 0n;
    const span = BigInt(NATIVE_HISTORY_PAGE_SIZE);
    let cursor =
      query.afterSeq ??
      (query.beforeSeq !== undefined
        ? query.beforeSeq > span + 1n
          ? query.beforeSeq - span - 1n
          : 0n
        : snapshotSeq > span
          ? snapshotSeq - span
          : 0n);
    let events: SessionEvent[] = [];
    let advanced = cursor;
    do {
      const page = await this.nativePage(cursor, signal);
      const rows = page.events.filter(
        (e) =>
          (query.beforeSeq === undefined || e.seq < query.beforeSeq) &&
          (snapshotSeq === 0n || e.seq <= snapshotSeq),
      );
      events = mergeEvents(events, rows);
      advanced = page.events.at(-1)?.seq ?? cursor;
      if (
        advanced <= cursor ||
        !page.events.length ||
        (query.beforeSeq !== undefined && advanced >= query.beforeSeq - 1n) ||
        (snapshotSeq > 0n && advanced >= snapshotSeq) ||
        (query.afterSeq !== undefined && query.beforeSeq === undefined)
      )
        break;
      cursor = advanced;
    } while (!signal.aborted);
    const first = events[0]?.seq ?? 0n;
    const last = events.at(-1)?.seq ?? advanced;
    return {
      events,
      metadata: events,
      snapshotSeq: last,
      hasOlder: first > 1n,
      hasNewer: snapshotSeq > last,
    };
  }
  readonly details: LoadEventDetails = async (seq, signal) => {
    const page = await this.sessions.eventDetails(
      { ref: ref(this.id), seq },
      { signal },
    );
    return page.events;
  };
}
