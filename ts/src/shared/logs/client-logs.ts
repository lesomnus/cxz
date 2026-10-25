import {
  logEntry,
  type LogEntry,
  type LogInput,
  type LogPage,
  type LogQuery,
} from "./log-entry";
import { MemoryLogStorage, type LogStorage } from "./log-storage";
import type { WorkerRequest } from "./log-worker";

type Command = WorkerRequest extends infer R
  ? R extends WorkerRequest
    ? Omit<R, "id">
    : never
  : never;
class WorkerLogStorage implements LogStorage {
  private closed = false;
  private sequence = 0;
  private pending = new Map<
    number,
    {
      resolve: (result: unknown) => void;
      reject: (error: Error) => void;
      timer: ReturnType<typeof setTimeout>;
    }
  >();
  constructor(private worker: Worker) {
    worker.onmessage = ({ data }) => {
      const call = this.pending.get(data.id);
      if (!call) return;
      this.pending.delete(data.id);
      clearTimeout(call.timer);
      if (data.error) call.reject(new Error(data.error));
      else call.resolve(data.result);
    };
    worker.onerror = () => this.close();
  }
  close() {
    this.closed = true;
    this.worker.terminate();
    for (const call of this.pending.values()) {
      clearTimeout(call.timer);
      call.reject(new Error("Log worker unavailable"));
    }
    this.pending.clear();
  }
  private call<T>(command: Command): Promise<T> {
    return new Promise((resolve, reject) => {
      if (this.closed) {
        reject(new Error("Log worker unavailable"));
        return;
      }
      const id = ++this.sequence;
      const timer = setTimeout(() => {
        this.close();
        reject(new Error("Log storage timed out"));
      }, 15000);
      this.pending.set(id, {
        resolve: (value) => resolve(value as T),
        reject,
        timer,
      });
      try {
        this.worker.postMessage({ ...command, id });
      } catch {
        this.close();
      }
    });
  }
  append(entries: LogEntry[]) {
    return this.call<void>({ action: "append", entries });
  }
  query(query: LogQuery) {
    return this.call<LogPage>({ action: "query", query });
  }
  export(query: LogQuery) {
    return this.call<string>({ action: "export", query });
  }
  clear() {
    return this.call<void>({ action: "clear" });
  }
}

export class ClientLogs {
  readonly tab = crypto.randomUUID();
  private buffer: LogEntry[] = [];
  private timer?: ReturnType<typeof setTimeout>;
  private work = Promise.resolve();
  private flushing = false;
  private dropped = 0;
  private listeners = new Set<() => void>();
  private channel?: BroadcastChannel;
  private store: LogStorage;
  constructor(
    store?: LogStorage,
    readonly version = "development",
  ) {
    this.store = store ?? new MemoryLogStorage();
    if (!store && typeof window !== "undefined") {
      try {
        this.store = new WorkerLogStorage(
          new Worker(new URL("./log-worker.ts", import.meta.url), {
            type: "module",
          }),
        );
      } catch {
        /* The visible memory-only mode still works without Workers. */
      }
      try {
        this.channel = new BroadcastChannel("cxz.client-logs.v1");
        this.channel.onmessage = ({ data }) => {
          if (data === "cleared") this.buffer = [];
          this.changed();
        };
      } catch {
        /* Single-tab diagnostics remain available. */
      }
    }
  }
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private changed() {
    this.listeners.forEach((listener) => {
      try {
        listener();
      } catch {
        /* A view cannot poison the storage queue. */
      }
    });
  }
  private async using<T>(operation: (store: LogStorage) => Promise<T>) {
    const store = this.store;
    try {
      return await operation(store);
    } catch {
      if (this.store === store) this.store = new MemoryLogStorage();
      return operation(this.store);
    }
  }
  write(input: LogInput) {
    try {
      if (this.buffer.length >= 256) {
        this.buffer.shift();
        this.dropped++;
      }
      this.buffer.push(logEntry(input, this.tab, this.version));
      this.timer ??= setTimeout(() => {
        void this.flush();
      }, 500);
    } catch {
      /* Observability must never fail the operation being observed. */
    }
  }
  flush() {
    clearTimeout(this.timer);
    this.timer = undefined;
    if (this.flushing || !this.buffer.length) return this.work;
    this.flushing = true;
    this.work = this.work
      .then(async () => {
        while (this.buffer.length) {
          const entries = this.buffer.splice(0);
          if (this.dropped) {
            entries.unshift(
              logEntry(
                {
                  level: "warn",
                  category: "client",
                  message: "Diagnostic records dropped while storage was busy",
                  details: { count: this.dropped },
                },
                this.tab,
                this.version,
              ),
            );
            this.dropped = 0;
          }
          await this.using((store) => store.append(entries));
          this.changed();
          this.channel?.postMessage("changed");
        }
      })
      .finally(() => {
        this.flushing = false;
      });
    return this.work;
  }
  async query(query: LogQuery = {}) {
    await this.flush();
    return this.using((store) => store.query(query));
  }
  async export(query: LogQuery = {}) {
    await this.flush();
    return this.using((store) => store.export(query));
  }
  async clear() {
    await this.flush();
    this.work = this.work.then(() => this.using((store) => store.clear()));
    await this.work;
    this.changed();
    this.channel?.postMessage("cleared");
  }
}

let singleton: ClientLogs | undefined;
export function clientLogs() {
  return (singleton ??= new ClientLogs(
    undefined,
    typeof __CXZ_CLIENT_REVISION__ === "string"
      ? __CXZ_CLIENT_REVISION__
      : "development",
  ));
}
export function recordClientError(error: unknown, message?: string) {
  clientLogs().write({
    level: "error",
    category: "client",
    message:
      message ??
      (error instanceof Error
        ? error.message
        : typeof error === "string"
          ? error
          : "Client error"),
    details: {
      error: error instanceof Error ? error.message : undefined,
      stack: error instanceof Error ? error.stack : undefined,
    },
  });
}
export function installClientLogs() {
  const logs = clientLogs();
  const error = (event: ErrorEvent) =>
    logs.write({
      level: "error",
      category: "client",
      message: event.message || "Uncaught JavaScript error",
      details: {
        stack: event.error instanceof Error ? event.error.stack : undefined,
        source: event.filename,
        line: event.lineno,
        column: event.colno,
      },
    });
  const rejection = (event: PromiseRejectionEvent) =>
    logs.write({
      level: "error",
      category: "client",
      message:
        event.reason instanceof Error
          ? event.reason.message
          : typeof event.reason === "string"
            ? event.reason
            : "Unhandled promise rejection",
      details: {
        stack: event.reason instanceof Error ? event.reason.stack : undefined,
      },
    });
  const flush = () => {
    void logs.flush();
  };
  window.addEventListener("error", error);
  window.addEventListener("unhandledrejection", rejection);
  window.addEventListener("pagehide", flush);
  document.addEventListener("visibilitychange", flush);
  logs.write({
    level: "info",
    category: "client",
    message: "UI client started",
  });
  return () => {
    flush();
    window.removeEventListener("error", error);
    window.removeEventListener("unhandledrejection", rejection);
    window.removeEventListener("pagehide", flush);
    document.removeEventListener("visibilitychange", flush);
  };
}
