import {
  MemoryLogStorage,
  OPFSLogStorage,
  type LogStorage,
} from "./log-storage";
import type { LogEntry, LogQuery } from "./log-entry";

export type WorkerRequest =
  | { id: number; action: "append"; entries: LogEntry[] }
  | { id: number; action: "query" | "export"; query: LogQuery }
  | { id: number; action: "clear" };

const memory = new MemoryLogStorage();
let store: LogStorage = memory;
let initialized = false;
let queue = Promise.resolve();
self.onmessage = ({ data }: MessageEvent<WorkerRequest>) => {
  // Async onmessage handlers overlap unless explicitly serialized.
  queue = queue.then(async () => {
    try {
      if (!initialized) {
        initialized = true;
        if (navigator.storage?.getDirectory && navigator.locks) {
          try {
            store = new OPFSLogStorage(
              await navigator.storage.getDirectory(),
              navigator.locks,
            );
          } catch {
            /* Restricted browser storage uses the bounded memory store. */
          }
        }
      }
      const operation = async () => {
        if (data.action === "append") return store.append(data.entries);
        if (data.action === "clear") return store.clear();
        if (data.action === "export") return store.export(data.query);
        return store.query(data.query);
      };
      let result: unknown;
      try {
        result = await operation();
      } catch {
        // Storage denial/quota errors must not break RPC or recursively log
        // storage errors. The page reports memory-only mode on its next read.
        store = memory;
        result = await operation();
      }
      self.postMessage({ id: data.id, result });
    } catch (error) {
      self.postMessage({ id: data.id, error: String(error) });
    }
  });
};
