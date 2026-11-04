import { useEffect, useRef, useState, type DependencyList } from "react";
import type { RemoteReadOptions } from "./read-cache";

// Reads belong to the currently selected resource. Late replies cannot replace
// a newer selection, and leaving a view cancels its request.
export function useRemote<T>(
  read: (signal: AbortSignal) => Promise<T>,
  deps: DependencyList,
  options: RemoteReadOptions = {},
) {
  const latest = useRef(read);
  latest.current = read;
  const [revision, setRevision] = useState(0);
  const previousDeps = useRef<DependencyList | undefined>(undefined);
  const previousRefresh = useRef(options.refresh);
  const cache = options.cache;
  const [state, setState] = useState<{
    data?: T;
    loading: boolean;
    error: string;
  }>(() => {
    const hit = cache?.store.peek<T>(cache.key);
    return {
      data: hit?.data,
      loading: options.enabled !== false && !hit?.fresh,
      error: "",
    };
  });
  useEffect(() => {
    const controller = new AbortController();
    const resourceDeps = [...deps, cache?.store, cache?.key];
    const sameResource =
      previousDeps.current?.length === resourceDeps.length &&
      resourceDeps.every((value, index) =>
        Object.is(value, previousDeps.current?.[index]),
      );
    previousDeps.current = resourceDeps;
    if (sameResource && previousRefresh.current !== options.refresh)
      cache?.store.invalidate(cache.key);
    previousRefresh.current = options.refresh;
    const hit = cache?.store.peek<T>(cache.key);
    if (options.enabled === false) {
      setState((old) => ({
        data: sameResource ? old.data : hit?.data,
        loading: false,
        error: "",
      }));
      return () => controller.abort();
    }
    if (hit?.fresh) {
      setState({ data: hit.data, loading: false, error: "" });
      return () => controller.abort();
    }
    setState((old) => ({
      data: hit?.data ?? (sameResource ? old.data : undefined),
      loading: true,
      error: "",
    }));
    const read = cache
      ? cache.store.read(
          cache.key,
          cache.ttl,
          latest.current,
          controller.signal,
        )
      : latest.current(controller.signal);
    read.then(
      (data) => {
        if (!controller.signal.aborted)
          setState({ data, loading: false, error: "" });
      },
      (error) => {
        if (!controller.signal.aborted)
          setState((old) => ({
            data: cache ? old.data : undefined,
            loading: false,
            error: String(error),
          }));
      },
    );
    return () => controller.abort();
  }, [
    ...deps,
    revision,
    cache?.store,
    cache?.key,
    options.refresh,
    options.enabled,
  ]);
  return {
    ...state,
    reload: () => {
      cache?.store.invalidate(cache.key);
      setRevision((n) => n + 1);
    },
    revalidate: () => setRevision((n) => n + 1),
  };
}
