import { useCallback, useEffect, useRef, useState } from "react";
import type { ClientLogs } from "#src/shared/logs/client-logs.ts";
import type { LogPage, LogQuery } from "#src/shared/logs/log-entry.ts";

export function mergeLogHead(
  current: LogPage | undefined,
  head: LogPage,
): LogPage {
  if (!current || !head.entries.length) return head;
  const ids = new Set(head.entries.map((entry) => entry.id));
  if (!current.entries.some((entry) => ids.has(entry.id))) return head;
  const entries = [
    ...head.entries,
    ...current.entries.filter((entry) => !ids.has(entry.id)),
  ].slice(0, head.total);
  return {
    ...head,
    entries,
    next: entries.length < head.total ? current.next : undefined,
  };
}

/** Follow the head while visible; older pages use exclusive, stable ID cursors. */
export function useLogFeed(logs: ClientLogs, query: LogQuery, live: boolean) {
  const [state, setState] = useState<{
    page?: LogPage;
    loading: boolean;
    error: string;
  }>({ loading: true, error: "" });
  const liveRef = useRef(live);
  liveRef.current = live;
  const controls = useRef({
    more() {},
    refresh() {},
    atTop(_value: boolean) {},
  });
  const signature = JSON.stringify(query);
  useEffect(() => {
    let disposed = false;
    let pending = false;
    let dirty = false;
    let atTop = true;
    let current: LogPage | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const filter: LogQuery = JSON.parse(signature);
    const publish = (loading: boolean, error = "") => {
      if (!disposed) setState({ page: current, loading, error });
    };
    async function read(older = false) {
      if (disposed) return;
      if (pending) {
        if (!older) dirty = true;
        return;
      }
      if (older && !current?.next) return;
      pending = true;
      if (!older) dirty = false;
      publish(true);
      try {
        const page = await logs.query({
          ...filter,
          before: older ? current?.next : undefined,
        });
        if (disposed) return;
        if (!older && current && !atTop) {
          dirty = true;
          publish(false);
          return;
        }
        if (older && current) {
          const ids = new Set(current.entries.map((entry) => entry.id));
          current = {
            ...page,
            entries: [
              ...current.entries,
              ...page.entries.filter((entry) => !ids.has(entry.id)),
            ],
          };
        } else current = mergeLogHead(current, page);
        publish(false);
      } catch (error) {
        publish(false, String(error));
      } finally {
        pending = false;
        if (!disposed && dirty && liveRef.current && atTop) void read();
      }
    }
    controls.current = {
      more: () => void read(true),
      refresh: () => void read(),
      atTop(value) {
        atTop = value;
        if (value && dirty && liveRef.current) void read();
      },
    };
    publish(true);
    void read();
    const unsubscribe = logs.subscribe(() => {
      if (!liveRef.current) return;
      dirty = true;
      if (atTop)
        timer ??= setTimeout(() => {
          timer = undefined;
          if (atTop && liveRef.current) void read();
        }, 200);
    });
    return () => {
      disposed = true;
      clearTimeout(timer);
      unsubscribe();
    };
  }, [logs, signature]);
  return {
    ...state,
    more: useCallback(() => controls.current.more(), []),
    refresh: useCallback(() => controls.current.refresh(), []),
    atTop: useCallback((value: boolean) => controls.current.atTop(value), []),
  };
}
