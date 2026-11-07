import { Code, ConnectError } from "@connectrpc/connect";
import { diagnosticText, type LogContext } from "#src/shared/logs/log-entry.ts";

export type ClientError = LogContext & {
  id: string;
  key: string;
  message: string;
  code?: string;
  target?: string;
  source: "rpc" | "http" | "client" | "resource";
  status: "active" | "resolved" | "dismissed" | "stale";
  firstSeen: number;
  lastSeen: number;
  updated: number;
  closedAt?: number;
  count: number;
};
export type ErrorContext = LogContext & {
  source?: ClientError["source"];
  code?: string;
  target?: string;
};
type StorageLike = Pick<Storage, "getItem" | "setItem">;
export const errorStorageKey = "cxz.client-errors.v1";
export const retainedErrors = 200;
export const errorStaleAfter = 60 * 60_000;

function errorMessage(error: unknown) {
  return diagnosticText(
    error instanceof ConnectError
      ? error.rawMessage
      : error instanceof Error
        ? error.message
        : String(error),
  );
}
export function isCancellation(error: unknown) {
  return (
    (error instanceof Error && error.name === "AbortError") ||
    (error instanceof ConnectError && error.code === Code.Canceled)
  );
}

/** Browser diagnostics, never requests, response bodies, headers or credentials. */
export class ClientErrors {
  private entries: readonly ClientError[] = [];
  private listeners = new Set<() => void>();
  private timer?: ReturnType<typeof setTimeout>;
  private expiryTimer?: ReturnType<typeof setTimeout>;
  private clearedBefore = 0;
  private clock = 0;
  private apiMessages = new Set<string>();
  private remember(message: string, source: ClientError["source"]) {
    if (source === "client") return;
    this.apiMessages.add(message.trim());
    if (this.apiMessages.size > 400)
      this.apiMessages.delete(this.apiMessages.values().next().value!);
  }
  private readonly storage?: StorageLike;
  readonly storageKey: string;
  constructor(
    options: {
      storage?: StorageLike;
      storageKey?: string;
      limit?: number;
    } = {},
  ) {
    this.storage = options.storage;
    this.storageKey = options.storageKey ?? errorStorageKey;
    this.limit = Math.max(1, Math.min(500, options.limit ?? retainedErrors));
    this.restore();
  }
  private readonly limit: number;
  snapshot = () => this.entries;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    this.expire();
    return () => {
      this.listeners.delete(listener);
      if (!this.listeners.size) clearTimeout(this.expiryTimer);
    };
  };
  private publish(entries: readonly ClientError[]) {
    this.entries = [...entries]
      .filter(
        (entry) =>
          entry.status === "active" || entry.updated > this.clearedBefore,
      )
      .sort(
        (a, b) =>
          Number(b.status === "active") - Number(a.status === "active") ||
          b.updated - a.updated,
      )
      .slice(0, this.limit);
    this.scheduleExpiry();
    for (const listener of this.listeners) {
      try {
        listener();
      } catch {
        /* Diagnostics cannot fail the observed operation. */
      }
    }
  }
  private scheduleExpiry() {
    clearTimeout(this.expiryTimer);
    this.expiryTimer = undefined;
    if (!this.listeners.size) return;
    const deadlines = this.entries
      .filter((entry) => entry.status === "active")
      .map((entry) => entry.lastSeen + errorStaleAfter);
    if (!deadlines.length) return;
    this.expiryTimer = setTimeout(
      () => this.expire(),
      Math.min(2_147_483_647, Math.max(1, Math.min(...deadlines) - Date.now())),
    );
  }
  private expire() {
    const now = Date.now();
    if (
      !this.entries.some(
        (entry) =>
          entry.status === "active" && entry.lastSeen + errorStaleAfter <= now,
      )
    ) {
      this.scheduleExpiry();
      return;
    }
    const updated = this.stamp();
    this.changed(
      this.entries.map((entry) =>
        entry.status === "active" && entry.lastSeen + errorStaleAfter <= now
          ? {
              ...entry,
              status: "stale",
              closedAt: entry.lastSeen + errorStaleAfter,
              updated,
            }
          : entry,
      ),
    );
  }
  private stamp() {
    return (this.clock = Math.max(Date.now(), this.clock + 1));
  }
  private changed(entries: readonly ClientError[]) {
    this.publish(entries);
    if (this.storage) this.timer ??= setTimeout(() => this.flush(), 250);
  }
  restore() {
    if (!this.storage) return;
    try {
      const data = JSON.parse(this.storage.getItem(this.storageKey) ?? "null");
      if (!data || !Array.isArray(data.entries)) return;
      this.clearedBefore = Math.max(
        this.clearedBefore,
        Number.isFinite(data.clearedBefore) ? data.clearedBefore : 0,
      );
      this.clock = Math.max(this.clock, this.clearedBefore);
      const merged = new Map(this.entries.map((entry) => [entry.id, entry]));
      for (const input of data.entries.slice(0, 500)) {
        if (
          !input ||
          typeof input.id !== "string" ||
          typeof input.key !== "string" ||
          typeof input.message !== "string" ||
          !["active", "resolved", "dismissed", "stale"].includes(
            input.status,
          ) ||
          !["rpc", "http", "client", "resource"].includes(input.source) ||
          !Number.isFinite(input.firstSeen) ||
          !Number.isFinite(input.lastSeen) ||
          !Number.isFinite(input.updated)
        )
          continue;
        const entry: ClientError = {
          id: input.id.slice(0, 64),
          key: diagnosticText(input.key),
          message: diagnosticText(input.message),
          status: input.status,
          source: input.source,
          firstSeen: input.firstSeen,
          lastSeen: input.lastSeen,
          updated: input.updated,
          count: Math.max(1, Math.min(1e9, Number(input.count) || 1)),
          closedAt: Number.isFinite(input.closedAt)
            ? input.closedAt
            : undefined,
        };
        for (const field of [
          "connection",
          "method",
          "session",
          "project",
          "request",
          "code",
          "target",
        ] as const)
          if (typeof input[field] === "string")
            entry[field] = diagnosticText(input[field], 512);
        const old = merged.get(entry.id);
        this.remember(entry.message, entry.source);
        this.clock = Math.max(this.clock, entry.updated);
        if (
          !old ||
          entry.updated > old.updated ||
          (entry.updated === old.updated && old.status === "active")
        )
          merged.set(entry.id, entry);
      }
      this.publish([...merged.values()]);
      this.expire();
    } catch {
      /* A corrupt/disabled browser store must not break the app. */
    }
  }
  flush = () => {
    clearTimeout(this.timer);
    this.timer = undefined;
    if (!this.storage) return;
    try {
      this.restore();
      this.storage.setItem(
        this.storageKey,
        JSON.stringify({
          clearedBefore: this.clearedBefore,
          entries: this.entries,
        }),
      );
    } catch {
      /* The current tab remains usable in memory-only mode. */
    }
  };
  report(key: string, error: unknown, context: ErrorContext = {}) {
    if (isCancellation(error)) return;
    this.expire();
    key = diagnosticText(key);
    const now = Date.now();
    const active = this.entries.find(
      (entry) => entry.key === key && entry.status === "active",
    );
    const entry: ClientError = {
      id: active?.id ?? crypto.randomUUID(),
      key: diagnosticText(key),
      message: errorMessage(error),
      source: context.source ?? "client",
      status: "active",
      firstSeen: active?.firstSeen ?? now,
      lastSeen: now,
      updated: this.stamp(),
      count: (active?.count ?? 0) + 1,
    };
    for (const field of [
      "connection",
      "method",
      "session",
      "project",
      "request",
      "target",
    ] as const)
      if (context[field]) entry[field] = diagnosticText(context[field], 512);
    const code =
      context.code ??
      (error instanceof ConnectError ? Code[error.code] : undefined);
    if (code) entry.code = diagnosticText(code, 64);
    this.remember(entry.message, entry.source);
    this.changed([
      entry,
      ...this.entries.filter((item) => item.id !== entry.id),
    ]);
    return entry.id;
  }
  resolve(key: string) {
    this.expire();
    key = diagnosticText(key);
    if (
      !this.entries.some(
        (entry) => entry.key === key && entry.status === "active",
      )
    )
      return;
    const now = Date.now(),
      updated = this.stamp();
    this.changed(
      this.entries.map((entry) =>
        entry.key === key && entry.status === "active"
          ? { ...entry, status: "resolved", closedAt: now, updated }
          : entry,
      ),
    );
  }
  observe(key: string, error: string, context: ErrorContext) {
    this.expire();
    key = diagnosticText(key);
    if (!error) {
      this.resolve(key);
      return;
    }
    if (
      this.entries.some(
        (entry) =>
          entry.key === key &&
          entry.status === "active" &&
          entry.message === errorMessage(error),
      )
    )
      return;
    this.report(key, error, context);
  }
  dismiss(id: string) {
    this.expire();
    const now = Date.now(),
      updated = this.stamp();
    this.changed(
      this.entries.map((entry) =>
        entry.id === id && entry.status === "active"
          ? { ...entry, status: "dismissed", closedAt: now, updated }
          : entry,
      ),
    );
  }
  clearHistory() {
    this.expire();
    this.clearedBefore = this.stamp();
    this.changed(this.entries.filter((entry) => entry.status === "active"));
  }
  containsAPIMessage(message: string) {
    const text = diagnosticText(message).trim();
    return (
      !!text &&
      (this.apiMessages.has(text) ||
        this.apiMessages.has(text.replace(/^Error:\s*/, "")))
    );
  }
}

/** For failures detected above the transport, such as a silent stream. */
export function operationHealth(
  store: ClientErrors | undefined,
  key: string,
  context: ErrorContext,
  signal?: AbortSignal,
) {
  return (error?: unknown) => {
    if (!store || signal?.aborted) return;
    if (error === undefined) {
      store.resolve(key);
      return;
    }
    // The transport already owns failures it observed. Avoid a second card
    // when a reader propagates that same error through its health check.
    if (
      store
        .snapshot()
        .some(
          (entry) =>
            entry.source === "rpc" &&
            entry.method === context.method &&
            entry.connection ===
              diagnosticText(context.connection ?? "", 512) &&
            entry.message === errorMessage(error),
        )
    )
      return;
    store.report(key, error, { ...context, source: "resource" });
  };
}

let singleton: ClientErrors | undefined;
export function clientErrors() {
  if (!singleton) {
    let storage: StorageLike | undefined;
    try {
      if (typeof window !== "undefined") storage = window.localStorage;
    } catch {
      /* Browser policy. */
    }
    singleton = new ClientErrors({ storage });
    if (typeof window !== "undefined") {
      window.addEventListener("pagehide", singleton.flush);
      window.addEventListener("storage", (event) => {
        if (event.key === singleton?.storageKey) singleton.restore();
      });
    }
  }
  return singleton;
}

/** Input validation stays local; API diagnostics belong to the error center. */
export function isAPIError(error: unknown, store = clientErrors()) {
  if (!error) return false;
  return (
    error instanceof ConnectError ||
    /(?:^\s*|ConnectError:\s*)\[(?:canceled|unknown|invalid_argument|deadline_exceeded|not_found|already_exists|permission_denied|resource_exhausted|failed_precondition|aborted|out_of_range|unimplemented|internal|unavailable|data_loss|unauthenticated)\]/i.test(
      String(error),
    ) ||
    store.containsAPIMessage(String(error))
  );
}
