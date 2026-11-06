import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
} from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { sessionRef, sessionUUID } from "#src/shared/api/session-id.ts";
import { BackgroundJobs } from "./background-jobs";

export function useSessionBackground(
  c: Connection,
  session: Uint8Array | undefined,
  run: string,
  enabled: boolean,
) {
  const uuid = sessionUUID(session);
  const store = useMemo(() => new BackgroundJobs(run), [c, uuid, run, enabled]);
  const latest = useRef(store);
  latest.current = store;
  const available = useRef(enabled);
  available.current = enabled;
  const snapshot = useSyncExternalStore(store.subscribe, store.snapshot);
  const refresh = useRef<() => void>(() => {});
  const receive = useCallback((event: SessionEvent) => {
    if (available.current) latest.current.receive(event);
  }, []);
  useEffect(() => {
    if (!enabled || !run || !uuid || !session) return;
    let controller: AbortController | undefined;
    let disposed = false;
    const read = () => {
      controller?.abort();
      const request = new AbortController();
      controller = request;
      store.beginSnapshot();
      void c.sessions
        .background(
          { ref: sessionRef(session) },
          { signal: request.signal, timeoutMs: 10000 },
        )
        .then((reply) => {
          if (!disposed && !request.signal.aborted)
            store.seed(reply.data, reply.lastSeq);
        })
        .catch(() => {
          if (!disposed && !request.signal.aborted) store.cancelSnapshot();
        });
    };
    refresh.current = read;
    const visible = () => {
      if (document.visibilityState === "visible") read();
    };
    read();
    document.addEventListener("visibilitychange", visible);
    return () => {
      disposed = true;
      controller?.abort();
      store.cancelSnapshot();
      refresh.current = () => {};
      document.removeEventListener("visibilitychange", visible);
    };
  }, [c, uuid, run, store, enabled]);
  return {
    count: enabled ? snapshot.count : undefined,
    receive,
    refresh: useCallback(() => {
      if (available.current) refresh.current();
    }, []),
  };
}
