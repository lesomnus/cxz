import type { SessionEvent } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import { Code, ConnectError } from "@connectrpc/connect";
import { SessionHistory, type HistoryPage } from "./session-history";
import { isTranscriptEvent, mergeEvents, pendingAfter } from "./journal";
import { mergeMetadata } from "./session-metadata";
import { advanceTurn, snapshotTurn, type TurnProgress } from "./turn-progress";
import { transcriptEvents } from "./tool-activity";
import type { ConversationLoad } from "../conversation/conversation-loading";
import type { TranscriptViewport } from "../conversation/virtual-messages";
import { CachedSessionAux } from "./session-aux-cache";

export const MAX_IDLE_CONVERSATIONS = 8;
export const MAX_CONVERSATION_BYTES = 8 << 20;
export const MAX_IDLE_CONVERSATION_BYTES = 32 << 20;
type Update<T> = T | ((old: T) => T);
type Client = Pick<Connection, "sessions" | "clientId" | "attention">;
export type ConversationSnapshot = {
  events: SessionEvent[];
  metadata: SessionEvent[];
  pending: SessionEvent[];
  gap: boolean;
  status: string;
  loading?: ConversationLoad;
  executionState: string;
  turn: TurnProgress;
  queued: string;
};
const empty = (): ConversationSnapshot => ({
  events: [],
  metadata: [],
  pending: [],
  gap: false,
  status: "Connecting…",
  loading: { stage: "session" },
  executionState: "",
  turn: { runId: "", active: false },
  queued: "",
});
export const eventBytes = (e: SessionEvent) => {
  const tool = e.toolSummary,
    response = e.response;
  const toolChars = tool
    ? tool.name.length +
      tool.shell.length +
      tool.command.length +
      tool.state.length +
      tool.files.reduce(
        (n, file) =>
          n +
          file.path.length +
          file.movePath.length +
          file.action.length +
          file.measure.length +
          64,
        0,
      )
    : 0;
  const responseChars = response
    ? response.model.length +
      response.effort.length +
      response.modelSource.length +
      response.effortSource.length +
      response.turnId.length +
      response.phase.length
    : 0;
  return (
    2 *
      (e.kind.length +
        e.text.length +
        e.runId.length +
        e.requestId.length +
        toolChars +
        responseChars) +
    e.payload.byteLength +
    (response?.completionJson.byteLength ?? 0) +
    128
  );
};
function bounded(rows: SessionEvent[], edge: "older" | "newer" = "newer") {
  let size = 0;
  const ordered = edge === "older" ? rows : [...rows].reverse();
  let count = 0;
  for (const e of ordered) {
    const bytes = eventBytes(e);
    if (count && size + bytes > MAX_CONVERSATION_BYTES) break;
    size += bytes;
    count++;
  }
  return count === rows.length
    ? rows
    : edge === "older"
      ? rows.slice(0, count)
      : rows.slice(-count);
}

// A connection owns data and the native stream; a React view only subscribes.
// Neither the journal nor provider payloads are written to browser storage.
export class CachedConversation {
  private state = empty();
  private listeners = new Set<() => void>();
  private eventListeners = new Set<(event: SessionEvent) => void>();
  private reconnectListeners = new Set<() => void>();
  private controller?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  private stopped = false;
  private paused = false;
  private started = false;
  private ready = false;
  private cursor = 0n;
  private identity = "";
  private notificationFrame?: number;
  private notificationQueued = false;
  readonly history: SessionHistory;
  readonly aux: CachedSessionAux;
  readonly latestSeq = { current: 0n };
  readonly newest = { current: { anchor: 0n, through: 0n } };
  readonly floor = { current: 0n };
  readonly detached = { current: false };
  readonly follow = { current: true };
  readonly latestTravel = { current: { shown: false, distance: 0 } };
  readonly viewport: TranscriptViewport = {
    sizes: new Map(),
    knots: new Map(),
    width: 0,
    anchor: null,
  };
  preceding?: { before: bigint; through: bigint; event?: SessionEvent };
  private tail: SessionEvent[] = [];
  constructor(
    private client: Client,
    readonly id: string,
    private changed: () => void,
  ) {
    this.history = new SessionHistory(client.sessions, id);
    this.aux = new CachedSessionAux(client.sessions, id, changed);
  }
  snapshot = () => this.state;
  get viewed() {
    return this.listeners.size > 0 || this.aux.viewed;
  }
  get active() {
    return (
      this.aux.active ||
      this.state.turn.active ||
      this.state.pending.length > 0 ||
      ["working", "running", "waiting_input", "waiting"].includes(
        this.state.executionState,
      )
    );
  }
  get bytes() {
    return [
      ...new Set([
        ...this.state.events,
        ...this.tail,
        ...this.state.metadata,
        ...this.state.pending,
      ]),
    ].reduce((n, e) => n + eventBytes(e), this.aux.bytes);
  }
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    if (this.paused && !this.stopped) this.resume();
    else if (!this.started && !this.stopped) {
      this.started = true;
      void this.connect();
    }
    return () => {
      this.listeners.delete(listener);
      if (!this.listeners.size && this.notificationFrame !== undefined) {
        cancelAnimationFrame(this.notificationFrame);
        this.notificationFrame = undefined;
        this.notificationQueued = false;
      }
      this.changed();
    };
  };
  onEvent(listener: (event: SessionEvent) => void) {
    this.eventListeners.add(listener);
    return () => {
      this.eventListeners.delete(listener);
    };
  }
  onReconnect(listener: () => void) {
    this.reconnectListeners.add(listener);
    return () => {
      this.reconnectListeners.delete(listener);
    };
  }
  private patch(patch: Partial<ConversationSnapshot>) {
    if (
      this.stopped ||
      !Object.entries(patch).some(
        ([key, value]) =>
          this.state[key as keyof ConversationSnapshot] !== value,
      )
    )
      return;
    this.state = { ...this.state, ...patch };
    if (this.listeners.size && !this.notificationQueued) {
      // Live bursts should paint once per frame; hidden conversations keep
      // receiving data without scheduling any React/DOM work.
      this.notificationQueued = true;
      const notify = () => {
        this.notificationQueued = false;
        this.notificationFrame = undefined;
        if (!this.stopped) for (const listener of this.listeners) listener();
      };
      if (typeof requestAnimationFrame === "function")
        this.notificationFrame = requestAnimationFrame(notify);
      else queueMicrotask(notify);
    }
    // The registry only sweeps on release/completion, not for every token.
  }
  setEvents = (
    update: Update<SessionEvent[]>,
    edge: "older" | "newer" = "newer",
  ) => {
    const rows =
      typeof update === "function" ? update(this.state.events) : update;
    const events = bounded(rows, edge);
    if (events !== rows && edge === "older") this.detached.current = true;
    this.patch({ events });
  };
  setMetadata = (update: Update<SessionEvent[]>) =>
    this.patch({
      metadata:
        typeof update === "function" ? update(this.state.metadata) : update,
    });
  setPending = (update: Update<SessionEvent[]>) =>
    this.patch({
      pending:
        typeof update === "function" ? update(this.state.pending) : update,
    });
  setQueued = (update: Update<string>) =>
    this.patch({
      queued: typeof update === "function" ? update(this.state.queued) : update,
    });
  rememberPage(page: HistoryPage) {
    this.setMetadata((old) => mergeMetadata(old, page.metadata));
    const first = page.events[0]?.seq,
      last = page.events.at(-1)?.seq;
    if (first !== undefined) {
      if (!page.hasOlder) this.floor.current = first - 1n;
      this.preceding = {
        before: first,
        through: page.events.find((e) => e.kind === "input")?.seq ?? last! + 1n,
        event: page.precedingInput,
      };
    }
    if (last !== undefined && !page.hasNewer) {
      this.newest.current = { anchor: last, through: page.snapshotSeq };
      this.tail = bounded(page.events);
    }
  }
  latestPage(): HistoryPage | undefined {
    if (!this.ready || this.newest.current.through < this.latestSeq.current)
      return;
    return {
      events: this.tail,
      metadata: this.state.metadata,
      snapshotSeq: this.latestSeq.current,
      hasOlder: (this.tail[0]?.seq ?? 0n) > this.floor.current + 1n,
      hasNewer: false,
    };
  }
  private receive(e: SessionEvent) {
    this.client.attention.event(this.id, e);
    for (const listener of this.eventListeners) listener(e);
    const patch: Partial<ConversationSnapshot> = {
      turn: advanceTurn(this.state.turn, e, true),
      metadata: mergeMetadata(this.state.metadata, [e]),
    };
    if (e.kind === "state") patch.executionState = e.text;
    if (e.kind === "queued") patch.queued = e.text;
    if (e.kind === "input" && this.state.queued === e.text) patch.queued = "";
    this.cursor = e.seq > this.cursor ? e.seq : this.cursor;
    if (this.cursor > this.latestSeq.current)
      this.latestSeq.current = this.cursor;
    if (isTranscriptEvent(e) || e.kind === "approval_resolved") {
      this.tail = bounded(mergeEvents(this.tail, [e]));
      if (!this.detached.current) {
        const next = bounded(mergeEvents(this.state.events, [e]));
        if (
          !this.follow.current &&
          transcriptEvents(this.state.events).events.some(
            (row) => row.seq < (next[0]?.seq ?? 0n),
          )
        )
          this.detached.current = true;
        else patch.events = next;
      }
      patch.pending = pendingAfter(this.state.pending, [e]);
    }
    this.newest.current = {
      anchor: this.tail.at(-1)?.seq ?? 0n,
      through: this.cursor,
    };
    const wasActive = this.active;
    this.patch(patch);
    if (wasActive && !this.active) this.changed();
  }
  private async connect() {
    if (this.stopped || this.paused) return;
    const request = new AbortController();
    this.controller = request;
    const signal = request.signal;
    this.patch({
      status: "Connecting…",
      ...(!this.ready
        ? { loading: { stage: "session" } as ConversationLoad }
        : {}),
    });
    try {
      const session = await this.client.sessions.get(
        { ref: ref(this.id), select: { all: true } },
        { signal },
      );
      if (signal.aborted) return;
      const identity = Array.from(session.id).join("-");
      const snapshotSeq = session.status?.lastSeq ?? 0n;
      const identityChanged = !!this.identity && this.identity !== identity;
      let journalReset = false;
      // Catalog status can lag the native stream. Only the journal's own
      // snapshot fence proves a reset; a stale LastSeq must not erase a cache.
      if (
        !identityChanged &&
        this.ready &&
        snapshotSeq < this.cursor &&
        this.history.mode === "summary"
      ) {
        try {
          const tail = await this.client.sessions.transcript(
            { ref: ref(this.id), limit: 1 },
            { signal },
          );
          if (signal.aborted) return;
          journalReset = tail.snapshotSeq < this.cursor;
        } catch (error) {
          if (ConnectError.from(error).code !== Code.Unimplemented) throw error;
          this.history.mode = "legacy";
        }
      }
      if (identityChanged || journalReset) {
        this.aux.reset();
        this.cursor = 0n;
        this.history.mode = "unknown";
        this.ready = false;
        this.tail = [];
        this.latestSeq.current = 0n;
        this.floor.current = 0n;
        this.detached.current = false;
        this.follow.current = true;
        this.preceding = undefined;
        this.viewport.sizes.clear();
        this.viewport.knots.clear();
        this.viewport.anchor = null;
        this.patch({ ...empty(), loading: { stage: "history" } });
      }
      this.identity = identity;
      this.latestSeq.current =
        snapshotSeq > this.latestSeq.current
          ? snapshotSeq
          : this.latestSeq.current;
      let loaded: SessionEvent[] = [];
      if (!this.ready) {
        this.patch({ loading: { stage: "history" } });
        const page = await this.history.page({ snapshotSeq }, signal);
        if (signal.aborted) return;
        this.rememberPage(page);
        loaded = page.events;
        this.cursor = page.snapshotSeq;
        this.tail = bounded(loaded);
      } else {
        for (const listener of this.reconnectListeners) listener();
        do {
          const page = await this.history.nativePage(this.cursor, signal);
          if (signal.aborted) return;
          this.setMetadata((old) => mergeMetadata(old, page.events));
          if (this.cursor > 0n && page.events[0]?.seq > this.cursor + 1n)
            this.patch({ gap: true });
          const next = page.events.at(-1)?.seq ?? this.cursor;
          if (next <= this.cursor) break;
          loaded = mergeEvents(loaded, page.events);
          this.cursor = next;
        } while (this.cursor < snapshotSeq);
        this.tail = bounded(
          mergeEvents(
            this.tail,
            loaded.filter(
              (e) => isTranscriptEvent(e) || e.kind === "approval_resolved",
            ),
          ),
        );
      }
      for (const e of loaded) {
        if (this.ready) this.client.attention.event(this.id, e);
        for (const listener of this.eventListeners) listener(e);
      }
      const wasActive = this.active;
      this.patch({
        pending: pendingAfter(
          session.status?.pending ?? [],
          loaded,
          snapshotSeq,
        ),
        events: this.detached.current
          ? this.state.events
          : bounded(
              mergeEvents(
                this.state.events,
                loaded.filter(
                  (e) => isTranscriptEvent(e) || e.kind === "approval_resolved",
                ),
              ),
            ),
        executionState: session.status?.state ?? "",
        turn: snapshotTurn(session.status, loaded, this.state.turn),
        queued: session.status?.queued ?? "",
        loading: undefined,
        status: "Live",
      });
      this.ready = true;
      this.newest.current = {
        anchor: this.tail.at(-1)?.seq ?? 0n,
        through: this.cursor,
      };
      if (wasActive && !this.active) this.changed();
      if (signal.aborted) return;
      for await (const e of this.client.sessions.events(
        {
          ref: ref(this.id),
          afterSeq: this.cursor,
          clientId: this.client.clientId,
        },
        { signal },
      )) {
        if (signal.aborted) return;
        this.receive(e);
      }
    } catch (error) {
      if (this.stopped || this.paused || this.controller !== request) return;
      this.patch({
        status: `Disconnected · retrying: ${String(error)}`,
        ...(!this.ready
          ? {
              loading: {
                stage: "retrying",
                error: String(error),
              } as ConversationLoad,
            }
          : {}),
      });
      if (ConnectError.from(error).code === Code.NotFound) {
        this.patch({
          executionState: "stopped",
          pending: [],
          turn: { runId: this.state.turn.runId, active: false },
        });
        this.pause();
        this.changed();
        return;
      }
    } finally {
      if (!this.stopped && !this.paused && this.controller === request)
        this.timer = setTimeout(() => void this.connect(), 2000);
    }
  }
  reconnect() {
    this.aux.reconnect();
    if (this.started && !this.paused) this.controller?.abort();
  }
  pause() {
    this.aux.pause();
    this.paused = true;
    this.controller?.abort();
    clearTimeout(this.timer);
  }
  resume() {
    this.aux.resume();
    if (this.paused && !this.stopped) {
      this.paused = false;
      if (this.started) void this.connect();
    }
  }
  dispose() {
    this.aux.dispose();
    this.stopped = true;
    this.pause();
    if (this.notificationFrame !== undefined)
      cancelAnimationFrame(this.notificationFrame);
    this.notificationFrame = undefined;
    this.listeners.clear();
    this.eventListeners.clear();
    this.reconnectListeners.clear();
    this.state = empty();
    this.tail = [];
    this.viewport.sizes.clear();
    this.viewport.knots.clear();
  }
}

export class ConversationCache {
  private entries = new Map<string, CachedConversation>();
  private owners = 0;
  constructor(private client: Client) {}
  get(id: string) {
    let entry = this.entries.get(id);
    if (!entry) {
      entry = new CachedConversation(this.client, id, () => this.sweep());
      this.entries.set(id, entry);
    } else {
      this.entries.delete(id);
      this.entries.set(id, entry);
    }
    return entry;
  }
  private sweep() {
    let idle = [...this.entries.values()].filter((e) => !e.viewed && !e.active);
    let bytes = idle.reduce((n, e) => n + e.bytes, 0);
    while (
      idle.length > MAX_IDLE_CONVERSATIONS ||
      (idle.length > 0 && bytes > MAX_IDLE_CONVERSATION_BYTES)
    ) {
      const entry = idle.shift()!;
      bytes -= entry.bytes;
      this.delete(entry.id);
    }
  }
  delete(id: string) {
    this.entries.get(id)?.dispose();
    this.entries.delete(id);
  }
  private reconnect = () => {
    for (const e of this.entries.values()) e.reconnect();
  };
  private visible = () => {
    if (document.visibilityState === "visible") this.reconnect();
  };
  mount() {
    if (++this.owners === 1) {
      for (const entry of this.entries.values()) entry.resume();
      window.addEventListener("offline", this.reconnect);
      window.addEventListener("online", this.reconnect);
      document.addEventListener("visibilitychange", this.visible);
    }
    return () => {
      if (--this.owners !== 0) return;
      window.removeEventListener("offline", this.reconnect);
      window.removeEventListener("online", this.reconnect);
      document.removeEventListener("visibilitychange", this.visible);
      for (const entry of this.entries.values()) entry.pause();
    };
  }
  dispose() {
    globalThis.window?.removeEventListener("offline", this.reconnect);
    globalThis.window?.removeEventListener("online", this.reconnect);
    globalThis.document?.removeEventListener("visibilitychange", this.visible);
    for (const entry of this.entries.values()) entry.dispose();
    this.entries.clear();
  }
}
