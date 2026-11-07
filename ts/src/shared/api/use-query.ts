import {
  create,
  type DescMessage,
  type DescMethodUnary,
  type MessageInitShape,
  type MessageShape,
} from "@bufbuild/protobuf";
import { useEffect, useMemo, useRef, useSyncExternalStore } from "react";
import type { Queries, QueryOpts } from "@lesomnus/payday/query";

// The caller supplies its authenticated Connection's Queries. Payday owns keys,
// response retention, normalization, deduplication and background revalidation.
export function useConnectionQuery<
  I extends DescMessage,
  O extends DescMessage,
>(
  queries: Queries,
  method: DescMethodUnary<I, O>,
  input: MessageInitShape<I> | undefined,
  opts: QueryOpts = {},
) {
  const entry =
    input === undefined ? undefined : queries.get(method, input, opts);
  const pending = useRef(new Set<AbortController>());
  useEffect(
    () => () => {
      for (const controller of pending.current) controller.abort();
      pending.current.clear();
    },
    [queries, entry?.key],
  );
  const [subscribe, snapshot] = useMemo(
    () =>
      [
        (changed: () => void) =>
          entry ? queries.subscribe(entry.key, changed) : () => {},
        () =>
          input === undefined ? undefined : queries.get(method, input, opts),
      ] as const,
    [queries, entry?.key],
  );
  const state = useSyncExternalStore(subscribe, snapshot, snapshot);
  return {
    data: state?.data,
    loading: input !== undefined && !state?.data && state?.state !== "error",
    error: state?.error === undefined ? "" : String(state.error),
    reload: () => {
      if (input !== undefined)
        queries.forget(method, create(method.input, input));
    },
    refresh: (accept: (data: MessageShape<O>) => boolean = () => true) => {
      if (input === undefined) return Promise.resolve(undefined);
      const controller = new AbortController();
      pending.current.add(controller);
      return refreshQuery(
        queries,
        method,
        input,
        accept,
        controller.signal,
      ).finally(() => pending.current.delete(controller));
    },
  };
}

// A save waits until the retained Read answer covers its confirmed revision.
// Row normalization can notify before the full response (including body) lands.
export function refreshQuery<I extends DescMessage, O extends DescMessage>(
  queries: Queries,
  method: DescMethodUnary<I, O>,
  input: MessageInitShape<I>,
  accept: (data: MessageShape<O>) => boolean = () => true,
  signal?: AbortSignal,
): Promise<MessageShape<O>> {
  signal?.throwIfAborted();
  const request = create(method.input, input);
  const before = queries.get(method, input, { watch: false });
  return new Promise((resolve, reject) => {
    let off = () => {};
    const finish = (error?: unknown, data?: MessageShape<O>) => {
      clearTimeout(timer);
      off();
      signal?.removeEventListener("abort", abort);
      if (error !== undefined) reject(error);
      else resolve(data!);
    };
    const abort = () =>
      finish(
        signal?.reason ?? new DOMException("Query canceled", "AbortError"),
      );
    const timer = setTimeout(
      () => finish(new Error("The query refresh timed out.")),
      30000,
    );
    off = queries.subscribe(before.key, () => {
      const next = queries.get(method, input, { watch: false });
      if (next.rev <= before.rev) return;
      if (next.state === "error") finish(next.error);
      else if (next.data && accept(next.data)) finish(undefined, next.data);
    });
    signal?.addEventListener("abort", abort, { once: true });
    queries.forget(method, request);
  });
}
