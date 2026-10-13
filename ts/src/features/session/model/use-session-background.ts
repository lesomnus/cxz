import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
} from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";
import { BackgroundJobs } from "./background-jobs";

export function useSessionBackground(
  c: Connection,
  session: string,
  run: string,
) {
  const store = useMemo(() => new BackgroundJobs(run), [c, session, run]);
  const latest = useRef(store);
  latest.current = store;
  const snapshot = useSyncExternalStore(store.subscribe, store.snapshot);
  const refresh = useRef<() => void>(() => {});
  const receive = useCallback(
    (event: SessionEvent) => latest.current.receive(event),
    [],
  );
  useEffect(() => {
    if (!run) return;
    let controller: AbortController | undefined;
    let disposed = false;
    const read = () => {
      controller?.abort();
      const request = new AbortController();
      controller = request;
      store.beginSnapshot();
      void c.sessions
        .background(
          { ref: ref(session) },
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
  }, [c, session, run, store]);
  return {
    count: snapshot.count,
    receive,
    refresh: useCallback(() => refresh.current(), []),
  };
}
