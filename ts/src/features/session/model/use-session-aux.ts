import { useEffect, useState } from "react";
import type { AuxState } from "#gen/cxz/session_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { ref } from "#src/shared/api/connection.ts";

// AuxEvents pushes the session's aux state: the manager runs each task once and
// every client reads the same answer, so this subscribes rather than polls and
// a summary generated while nobody was looking is in the first frame.
//
// Failure is silent on purpose. A summary is beside the conversation, not part
// of it, and a session whose aux is unconfigured or whose manager predates the
// call must read exactly as it does now rather than carry an error banner.
export function useSessionAux(c: Connection, id: string) {
  const [state, setState] = useState<AuxState>();
  useEffect(() => {
    setState(undefined);
    if (!id) return;
    let canceled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let active: AbortController | undefined;
    async function connect() {
      const controller = new AbortController();
      active = controller;
      try {
        for await (const next of c.sessions.auxEvents(
          { ref: ref(id) },
          { signal: controller.signal },
        )) {
          if (canceled) return;
          setState(next);
        }
      } catch {
        /* Retried below; the conversation does not depend on this. */
      } finally {
        if (!canceled) timer = setTimeout(connect, 5000);
      }
    }
    void connect();
    // The stream is idle between turns, so a connection parked by a sleeping
    // tab is dropped and reopened rather than waited on.
    const resume = () => {
      if (document.visibilityState === "visible") active?.abort();
    };
    const offline = () => active?.abort();
    window.addEventListener("offline", offline);
    document.addEventListener("visibilitychange", resume);
    return () => {
      canceled = true;
      clearTimeout(timer);
      active?.abort();
      window.removeEventListener("offline", offline);
      document.removeEventListener("visibilitychange", resume);
    };
  }, [c, id]);
  return state;
}
