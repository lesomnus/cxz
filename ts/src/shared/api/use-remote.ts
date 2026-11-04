import { useEffect, useRef, useState, type DependencyList } from "react";
import type { RemoteReadOptions } from "./read-cache";

const sameDependencies = (left: DependencyList, right: DependencyList) =>
  left.length === right.length &&
  left.every((value, index) => Object.is(value, right[index]));

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
  const cache = options.cache;
  const resourceDeps = [...deps, cache?.store, cache?.key];
  const previousDeps = useRef<DependencyList>(resourceDeps);
  const previousRefresh = useRef(options.refresh);
  const previousRevision = useRef(revision);
  const previousEnabled = useRef(options.enabled);
  const waiting = useRef<(() => void)[]>([]);
  useEffect(
    () => () => {
      for (const resolve of waiting.current.splice(0)) resolve();
    },
    [],
  );
  const hit = cache?.store.peek<T>(cache.key);
  const [state, setState] = useState<{
    data?: T;
    /** Origin of the displayed data, even after a remote result populates the cache. */
    fromCache: boolean;
    loading: boolean;
    error: string;
  }>(() => {
    return {
      data: hit?.data,
      fromCache: !!hit,
      loading: options.enabled !== false && !hit?.fresh,
      error: "",
    };
  });
  useEffect(() => {
    const controller = new AbortController();
    const completions = waiting.current.splice(0);
    const finish = () => {
      for (const resolve of completions.splice(0)) resolve();
    };
    const cancel = () => {
      controller.abort();
      finish();
    };
    const sameResource = sameDependencies(previousDeps.current, resourceDeps);
    previousDeps.current = resourceDeps;
    previousRevision.current = revision;
    previousEnabled.current = options.enabled;
    if (sameResource && previousRefresh.current !== options.refresh)
      cache?.store.invalidate(cache.key);
    previousRefresh.current = options.refresh;
    const hit = cache?.store.peek<T>(cache.key);
    if (options.enabled === false) {
      setState((old) => ({
        data: sameResource ? old.data : hit?.data,
        fromCache: sameResource ? old.fromCache : !!hit,
        loading: false,
        error: "",
      }));
      finish();
      return cancel;
    }
    if (hit?.fresh) {
      setState({ data: hit.data, fromCache: true, loading: false, error: "" });
      finish();
      return cancel;
    }
    setState((old) => ({
      data:
        hit?.data ??
        (sameResource || options.keepPreviousData ? old.data : undefined),
      fromCache: hit
        ? true
        : sameResource || options.keepPreviousData
          ? old.fromCache
          : false,
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
          setState({ data, fromCache: false, loading: false, error: "" });
        finish();
      },
      (error) => {
        if (!controller.signal.aborted)
          setState((old) => ({
            data: cache ? old.data : undefined,
            fromCache: cache ? old.fromCache : false,
            loading: false,
            error: String(error),
          }));
        finish();
      },
    );
    return cancel;
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
    // Dependent requests become busy in the render that changes their inputs,
    // without an idle frame between a summary response and its calendar read.
    loading:
      options.enabled !== false &&
      (state.loading ||
        ((!sameDependencies(previousDeps.current, resourceDeps) ||
          previousRevision.current !== revision ||
          previousRefresh.current !== options.refresh ||
          previousEnabled.current !== options.enabled) &&
          (!hit?.fresh || previousRefresh.current !== options.refresh))),
    reload: () => {
      cache?.store.invalidate(cache.key);
      setState((old) => ({ ...old, loading: true, error: "" }));
      const done = new Promise<void>((resolve) =>
        waiting.current.push(resolve),
      );
      setRevision((n) => n + 1);
      return done;
    },
    revalidate: () => setRevision((n) => n + 1),
  };
}
