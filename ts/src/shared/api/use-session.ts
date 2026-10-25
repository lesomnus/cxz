import { useRef } from "react";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Connection } from "./connection";
import type { Session } from "#gen/cxz/session_pb";
import {
  matchesResource,
  resourceRef,
} from "#src/shared/navigation/resource-route.ts";
import { useRemote } from "./use-remote";

// The URL is a locator; conversation state remains keyed by the resolved runtime ID.
export function useSession(
  c: Connection,
  sessions: readonly Session[],
  handle: string,
) {
  const remembered = useRef<{ handle: string; id: string } | undefined>(
    undefined,
  );
  const found =
    sessions.find((s) => matchesResource(s, handle)) ??
    (remembered.current?.handle === handle
      ? sessions.find((s) => s.runtimeId === remembered.current?.id)
      : undefined);
  if (found) remembered.current = { handle, id: found.runtimeId };
  const remote = useRemote(
    async (signal) => {
      if (!handle || found) return;
      const ref = resourceRef(handle);
      let session: Session;
      try {
        session = await c.sessions.get(
          { ref, select: { all: true, project: { all: true } } },
          { signal, timeoutMs: 10000 },
        );
      } catch (error) {
        // Older bookmarks used the runtime handle instead of a resource alias.
        const code = ConnectError.from(error).code;
        if (
          ref.key.case !== "alias" ||
          (code !== Code.NotFound &&
            !(code === Code.InvalidArgument && /^[\da-f]{24}$/i.test(handle)))
        )
          throw error;
        session = await c.sessions.get(
          {
            ref: { key: { case: "runtimeId", value: handle } },
            select: { all: true, project: { all: true } },
          },
          { signal, timeoutMs: 10000 },
        );
      }
      c.store.apply("cxz.Session", [{ id: session.id, value: session }]);
      return { handle, session };
    },
    [c, handle, !!found],
  );
  const session =
    found ?? (remote.data?.handle === handle ? remote.data.session : undefined);
  return {
    session,
    loading: !!handle && !session && remote.loading,
    error: session ? "" : remote.error,
    reload: remote.reload,
  };
}
