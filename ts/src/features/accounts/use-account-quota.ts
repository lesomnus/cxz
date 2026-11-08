import { useEffect } from "react";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { accountReadPrefix, USAGE_TTL } from "#src/shared/api/read-cache.ts";
import { readAccountQuota } from "./account-quota";

/** Account cards and account selectors share the same bounded metadata read. */
export function useAccountQuota({
  c,
  agent,
  sessions,
  identity,
  refreshVersion = 0,
}: {
  c: Connection;
  agent: string;
  sessions: Session[];
  identity: string;
  refreshVersion?: number;
}) {
  const identities = JSON.stringify(
    sessions.map((session) => session.runtimeId).sort(),
  );
  const quota = useRemote(
    (signal) => readAccountQuota(c, agent, sessions, signal),
    [c, identity, agent, identities],
    {
      cache: {
        store: c.reads,
        key: `${accountReadPrefix(identity)}quota/${agent}/${identities}`,
        ttl: USAGE_TTL,
      },
      refresh: refreshVersion,
    },
  );
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible") quota.revalidate();
    };
    const timer = setInterval(refresh, 60000);
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      clearInterval(timer);
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [c, identity, agent, identities]);
  return quota;
}
