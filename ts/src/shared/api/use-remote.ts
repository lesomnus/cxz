import { useEffect, useRef, useState, type DependencyList } from "react";

// Reads belong to the currently selected resource. Late replies cannot replace
// a newer selection, and leaving a view cancels its request.
export function useRemote<T>(
  read: (signal: AbortSignal) => Promise<T>,
  deps: DependencyList,
) {
  const latest = useRef(read);
  latest.current = read;
  const [revision, setRevision] = useState(0);
  const previousDeps = useRef<DependencyList | undefined>(undefined);
  const [state, setState] = useState<{
    data?: T;
    loading: boolean;
    error: string;
  }>({ loading: true, error: "" });
  useEffect(() => {
    const controller = new AbortController();
    const sameResource =
      previousDeps.current?.length === deps.length &&
      deps.every((value, index) =>
        Object.is(value, previousDeps.current?.[index]),
      );
    previousDeps.current = deps;
    setState((old) => ({
      data: sameResource ? old.data : undefined,
      loading: true,
      error: "",
    }));
    latest.current(controller.signal).then(
      (data) => {
        if (!controller.signal.aborted)
          setState({ data, loading: false, error: "" });
      },
      (error) => {
        if (!controller.signal.aborted)
          setState({ loading: false, error: String(error) });
      },
    );
    return () => controller.abort();
  }, [...deps, revision]);
  return { ...state, reload: () => setRevision((n) => n + 1) };
}
