import { fromBinary } from "@bufbuild/protobuf";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import { SessionNotificationSchema } from "#gen/cxz/session_svc_pb";
import type { SessionService } from "#gen/cxz/session_svc_pb";
import type {
  NotificationService,
  NotificationReadItem,
} from "#gen/cxz/payday/notification_pb";

export type SessionAlert = "complete" | "question" | "error";
const kinds = ["session.completed", "session.failed", "session.question"];
const key = (id: Uint8Array) =>
  Array.from(id, (b) => b.toString(16).padStart(2, "0")).join("");
type Note = {
  id: string;
  session: string;
  generation: string;
  seq: bigint;
  kind: SessionAlert;
};
type Group = {
  topics: { key: { case: "resourceId"; value: Uint8Array } }[];
  abort: AbortController;
  unread: Note[];
  loading: boolean;
  dirty: boolean;
  announce: boolean;
  known: Set<string>;
  receipted: Set<string>;
};
type Ack = {
  wanted: bigint;
  confirmed: bigint;
  generation: string;
  optimistic: Map<string, bigint>;
  rejected: bigint;
  busy: boolean;
  abort: AbortController;
  timer?: ReturnType<typeof setTimeout>;
  last: number;
};

// The Manager owns occurrences and read state. This is a connection-local view
// of durable receipts, plus sound deduplication for the current browser tab.
export class SessionAttention {
  private sessions = new Map<string, Session>();
  private groups: Group[] = [];
  private topology = "";
  private reader = "";
  private acks = new Map<string, Ack>();
  private listeners = new Set<() => void>();
  private alerts = new Set<(kind: SessionAlert) => void>();
  private reading = { id: "", through: 0n };
  private value = new Set<string>();
  private errors = new Set<string>();
  private stopped = false;
  private unsupported = false;
  constructor(
    private api: Pick<Client<typeof SessionService>, "readState" | "markRead">,
    private notifications: Pick<
      Client<typeof NotificationService>,
      "read" | "listen"
    >,
  ) {}
  snapshot = () => this.value;
  errorSnapshot = () => this.errors;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  onAlert = (fn: (kind: SessionAlert) => void) => {
    this.alerts.add(fn);
    return () => {
      this.alerts.delete(fn);
    };
  };
  private publish() {
    const unread = this.groups
      .flatMap((g) => g.unread.filter((n) => !g.receipted.has(n.id)))
      .filter((n) => {
        if (!this.sessions.has(n.session)) return false;
        const ack = this.acks.get(n.session);
        if (ack && n.seq <= (ack.optimistic.get(n.generation) ?? 0n))
          return false;
        if (ack?.generation === n.generation && n.seq <= ack.confirmed)
          return false;
        return true;
      });
    const next = new Set(unread.map((n) => n.session));
    const errors = new Set(
      unread.filter((n) => n.kind === "error").map((n) => n.session),
    );
    if (
      next.size === this.value.size &&
      [...next].every((id) => this.value.has(id)) &&
      errors.size === this.errors.size &&
      [...errors].every((id) => this.errors.has(id))
    )
      return;
    this.value = next;
    this.errors = errors;
    this.listeners.forEach((fn) => fn());
  }
  observe(sessions: Session[]) {
    if (this.stopped || this.unsupported) return;
    this.sessions = new Map(sessions.map((s) => [s.runtimeId, s]));
    const topics = new Map<string, Uint8Array>();
    for (const s of sessions)
      if (s.project?.id.length) topics.set(key(s.project.id), s.project.id);
    const sorted = [...topics].sort(([a], [b]) => a.localeCompare(b));
    const topology = sorted.map(([id]) => id).join("/");
    if (topology !== this.topology) {
      this.groups.forEach((g) => g.abort.abort());
      this.topology = topology;
      this.groups = [];
      for (let i = 0; i < sorted.length; i += 100) {
        const group: Group = {
          topics: sorted
            .slice(i, i + 100)
            .map(([, value]) => ({ key: { case: "resourceId", value } })),
          abort: new AbortController(),
          unread: [],
          loading: false,
          dirty: false,
          announce: false,
          known: new Set(),
          receipted: new Set(),
        };
        this.groups.push(group);
        void this.watch(group);
      }
    }
    this.publish();
  }
  private acceptReader(id: Uint8Array) {
    const reader = key(id);
    if (this.reader && reader !== this.reader) {
      this.acks.forEach((a) => {
        a.abort.abort();
        clearTimeout(a.timer);
      });
      this.acks.clear();
      this.groups.forEach((g) => {
        g.unread = [];
        g.known.clear();
        g.receipted.clear();
      });
    }
    this.reader = reader;
  }
  private unavailable(error: unknown) {
    if (ConnectError.from(error).code !== Code.Unimplemented) return false;
    this.unsupported = true;
    this.clear();
    return true;
  }
  private async watch(group: Group) {
    let delay = 1000;
    while (!group.abort.signal.aborted) {
      try {
        for await (const frame of this.notifications.listen(
          { topics: group.topics },
          { signal: group.abort.signal },
        )) {
          if (frame.event.case === "ready") {
            this.acceptReader(frame.event.value.readerId);
            delay = 1000;
            await this.refresh(group, false);
          } else if (frame.event.case === "invalidated") {
            void this.refresh(group, true);
          }
        }
      } catch (error) {
        if (this.unavailable(error)) return;
        /* Reconcile durable state after reconnect; hints carry no history. */
      }
      if (group.abort.signal.aborted) return;
      await new Promise<void>((resolve) => {
        const timer = setTimeout(done, delay);
        function done() {
          clearTimeout(timer);
          group.abort.signal.removeEventListener("abort", done);
          resolve();
        }
        group.abort.signal.addEventListener("abort", done, { once: true });
      });
      delay = Math.min(delay * 2, 30000);
    }
  }
  private note(item: NotificationReadItem): Note | undefined {
    const value = item.notification;
    if (
      !value?.payload ||
      !kinds.includes(value.kind) ||
      value.payload.typeUrl !== "type.googleapis.com/cxz.SessionNotification"
    )
      return;
    try {
      const payload = fromBinary(
        SessionNotificationSchema,
        value.payload.value,
      );
      return {
        id: key(value.id),
        session: payload.runtimeId,
        generation: payload.generation,
        seq: payload.seq,
        kind:
          value.kind === "session.failed"
            ? "error"
            : value.kind === "session.question"
              ? "question"
              : "complete",
      };
    } catch {
      return;
    }
  }
  private async refresh(group: Group, announce: boolean) {
    if (group.abort.signal.aborted) return;
    group.dirty = true;
    group.announce ||= announce;
    if (group.loading) return;
    group.loading = true;
    try {
      while (group.dirty && !group.abort.signal.aborted) {
        const sound = group.announce;
        group.dirty = group.announce = false;
        const reader = this.reader;
        const options = { signal: group.abort.signal, timeoutMs: 5000 };
        const unread: Note[] = [];
        let cursor = "";
        do {
          const page = await this.notifications.read(
            {
              topics: group.topics,
              filter: { kinds, unreadOnly: true },
              size: 100,
              cursor,
            },
            options,
          );
          if (key(page.readerId) !== reader || group.abort.signal.aborted)
            return;
          for (const item of page.items) {
            const n = this.note(item);
            if (n) unread.push(n);
          }
          cursor = page.next;
        } while (cursor);
        // Include recent receipts: a live completion can already be read when
        // its hint arrives, and must still play its sound once.
        const recent = await this.notifications.read(
          { topics: group.topics, filter: { kinds }, size: 100 },
          options,
        );
        if (key(recent.readerId) !== reader || group.abort.signal.aborted)
          return;
        const arrivals = new Map<string, Note>();
        const receipted = new Set<string>();
        for (const item of recent.items) {
          const n = this.note(item);
          if (!n) continue;
          arrivals.set(n.id, n);
          if (item.receipt) receipted.add(n.id);
        }
        unread.forEach((n) => arrivals.set(n.id, n));
        for (const n of arrivals.values()) {
          if (sound && !group.known.has(n.id) && this.sessions.has(n.session))
            this.alerts.forEach((fn) => fn(n.kind));
        }
        for (const n of unread)
          if (group.receipted.has(n.id)) receipted.add(n.id);
        group.known = new Set(arrivals.keys());
        group.receipted = receipted;
        group.unread = unread;
        if (this.reading.id)
          this.queueAck(this.reading.id, this.reading.through);
        this.publish();
      }
    } catch (error) {
      if (this.unavailable(error)) return;
      // Failed reads retain the last successful projection.
      if (!group.abort.signal.aborted)
        setTimeout(() => {
          if (!group.abort.signal.aborted) void this.refresh(group, false);
        }, 2000);
    } finally {
      group.loading = false;
    }
  }
  setReading(id: string, through = 0n) {
    if (id && id !== this.reading.id) {
      const ack = this.acks.get(id);
      if (ack) ack.rejected = 0n;
    }
    this.reading = { id, through };
    if (id) this.queueAck(id, through);
    this.publish();
  }
  event(id: string, event: SessionEvent) {
    if (event.kind === "turn_end" && this.reading.id === id)
      this.queueAck(id, this.reading.through);
  }
  private queueAck(id: string, through: bigint) {
    if (!through || this.stopped || this.unsupported || !this.sessions.has(id))
      return;
    let ack = this.acks.get(id);
    if (!ack) {
      ack = {
        wanted: 0n,
        confirmed: 0n,
        generation: "",
        optimistic: new Map(),
        rejected: 0n,
        busy: false,
        abort: new AbortController(),
        last: 0,
      };
      this.acks.set(id, ack);
    }
    if (through <= ack.rejected) return;
    ack.wanted = through > ack.wanted ? through : ack.wanted;
    // Payload generations let the UI project a read before even ReadState
    // answers. Keep the projection while navigating away or retrying offline.
    if (ack.generation) {
      if (ack.wanted > ack.confirmed)
        ack.optimistic.set(ack.generation, ack.wanted);
    } else {
      for (const group of this.groups)
        for (const note of group.unread)
          if (
            note.session === id &&
            note.seq <= through &&
            through > (ack.optimistic.get(note.generation) ?? 0n)
          )
            ack.optimistic.set(note.generation, through);
    }
    if (ack.busy || ack.timer || ack.wanted <= ack.confirmed) return;
    ack.timer = setTimeout(
      () => {
        ack.timer = undefined;
        void this.acknowledge(id, ack);
      },
      Math.max(150, 1000 - (Date.now() - ack.last)),
    );
  }
  private async acknowledge(id: string, ack: Ack) {
    ack.busy = true;
    try {
      const options = { signal: ack.abort.signal, timeoutMs: 5000 };
      const ref = { key: { case: "runtimeId" as const, value: id } };
      if (!ack.generation) {
        const state = await this.api.readState({ ref }, options);
        if (ack.abort.signal.aborted) return;
        ack.generation = state.generation;
        ack.confirmed = state.throughSeq;
        for (const generation of ack.optimistic.keys())
          if (generation !== state.generation)
            ack.optimistic.delete(generation);
        if (ack.wanted > ack.confirmed)
          ack.optimistic.set(state.generation, ack.wanted);
        this.publish();
      }
      if (ack.wanted > ack.confirmed) {
        const state = await this.api.markRead(
          { ref, generation: ack.generation, throughSeq: ack.wanted },
          options,
        );
        if (ack.abort.signal.aborted) return;
        ack.confirmed =
          state.throughSeq > ack.confirmed ? state.throughSeq : ack.confirmed;
        ack.generation = state.generation;
      }
      for (const [generation, through] of ack.optimistic)
        if (generation !== ack.generation || through <= ack.confirmed)
          ack.optimistic.delete(generation);
      this.publish();
    } catch (error) {
      if (this.unavailable(error)) return;
      const code = ConnectError.from(error).code;
      if (
        [Code.FailedPrecondition, Code.OutOfRange, Code.NotFound].includes(code)
      ) {
        ack.generation = "";
        ack.confirmed = 0n;
      }
      if (
        [
          Code.Unauthenticated,
          Code.PermissionDenied,
          Code.InvalidArgument,
          Code.FailedPrecondition,
          Code.OutOfRange,
          Code.NotFound,
        ].includes(code)
      ) {
        ack.rejected = ack.wanted;
        ack.wanted = ack.confirmed;
        ack.optimistic.clear();
        this.publish();
      }
    } finally {
      ack.busy = false;
      ack.last = Date.now();
      if (!ack.abort.signal.aborted && ack.wanted > ack.confirmed)
        this.queueAck(id, ack.wanted);
    }
  }
  reset(id: string) {
    const ack = this.acks.get(id);
    if (ack) {
      ack.abort.abort();
      clearTimeout(ack.timer);
      this.acks.delete(id);
    }
    this.groups.forEach((g) => {
      g.unread = g.unread.filter((n) => n.session !== id);
    });
    if (this.reading.id === id) this.reading = { id: "", through: 0n };
    this.publish();
  }
  clear() {
    this.groups.forEach((g) => g.abort.abort());
    this.groups = [];
    this.topology = "";
    this.acks.forEach((a) => {
      a.abort.abort();
      clearTimeout(a.timer);
    });
    this.acks.clear();
    this.sessions.clear();
    this.reader = "";
    this.reading = { id: "", through: 0n };
    this.publish();
  }
  dispose() {
    this.stopped = true;
    this.clear();
  }
}
