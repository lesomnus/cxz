import { useEffect, useMemo, useState } from "react";
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import { key } from "@lesomnus/payday/store";
import type { Project } from "#gen/cxz/project_pb";
import type { SessionRef } from "#gen/cxz/session_svc_pb";
import type { Memory } from "#gen/cxz/memory_pb";
import { MemoryScope, type MemoryService } from "#gen/cxz/memory_svc_pb";
import { projectId, projectRef } from "#src/shared/api/project-id.ts";
import { readBy, release } from "#src/shared/api/stream-deadline.ts";

export type WorkMemorySource = {
  project: Project;
  session: SessionRef;
  memories: Pick<Client<typeof MemoryService>, "watch">;
};
export type WorkMemoryState = {
  memories: Memory[];
  loading: boolean;
  error: string;
  refresh: () => void;
};
export const emptyWorkMemories: WorkMemoryState = {
  memories: [],
  loading: false,
  error: "",
  refresh: () => {},
};

/** Server snapshots and changes contain metadata only, never document bodies. */
export async function watchWorkMemories(
  source: WorkMemorySource,
  signal: AbortSignal,
  receive: (memories: Memory[]) => void,
) {
  const rows = new Map<string, Memory>();
  const stream = source.memories
    .watch(
      {
        project: projectRef(projectId(source.project)),
        session: source.session,
        scope: MemoryScope.ACTIVE,
      },
      { signal, timeoutMs: 0 },
    )
    [Symbol.asyncIterator]();
  try {
    // Unlike Task.Watch, Memory.Watch has no changes-only readiness frame.
    // Its initial metadata snapshot is the baseline; quiet updates are valid.
    let next = await readBy(
      stream.next(),
      15_000,
      "Memory subscription did not return its initial snapshot.",
    );
    while (!next.done && !signal.aborted) {
      for (const item of next.value.items) {
        const id = key(item.id);
        if (item.value && !item.value.dateDeleted) rows.set(id, item.value);
        else rows.delete(id);
      }
      receive([...rows.values()]);
      if (signal.aborted) break;
      next = await stream.next();
    }
    if (!signal.aborted)
      throw new ConnectError(
        "Memory subscription disconnected.",
        Code.Unavailable,
      );
  } finally {
    release(stream);
  }
}

/** Memory.Watch supplies one metadata snapshot, then changes, for all ranges. */
export function useWorkMemories(source?: WorkMemorySource): WorkMemoryState {
  const [revision, refresh] = useState(0);
  const [state, setState] = useState({ source, ...emptyWorkMemories });
  useEffect(() => {
    if (!source) return;
    const controller = new AbortController();
    const { signal } = controller;
    setState((previous) => ({
      ...previous,
      source,
      memories: previous.source === source ? previous.memories : [],
      loading: true,
      error: "",
    }));
    void watchWorkMemories(source, signal, (memories) => {
      setState({
        source,
        memories,
        loading: false,
        error: "",
        refresh: () => {},
      });
    }).catch((error) => {
      if (!signal.aborted)
        setState((previous) => ({
          ...previous,
          loading: false,
          error: String(error),
        }));
      controller.abort();
    });
    return () => controller.abort();
  }, [source, revision]);
  return useMemo(
    () => ({
      ...(state.source === source
        ? state
        : { ...emptyWorkMemories, loading: !!source }),
      refresh: () => refresh((value) => value + 1),
    }),
    [state, source],
  );
}
