import { useEffect, useRef, useState } from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { isQuestionEvent } from "./journal";

const approvalDisplayDelay = 150;
const identity = (event: SessionEvent) =>
  `${event.runId}:${event.requestId}:${event.seq}`;

// Automatic approvals remain in the journal, but usually resolve before the
// next paint. Only unresolved permission requests should open a card. Human
// questions bypass this brief grace period and appear immediately.
export function useVisibleApprovals(pending: SessionEvent[]) {
  const waiting = useRef(
    new Map<
      string,
      {
        timer: ReturnType<typeof setTimeout>;
        ready: boolean;
      }
    >(),
  );
  const [, redraw] = useState(0);
  useEffect(() => {
    const keys = new Set(
      pending.filter((event) => !isQuestionEvent(event)).map(identity),
    );
    for (const [key, entry] of waiting.current) {
      if (!keys.has(key)) {
        clearTimeout(entry.timer);
        waiting.current.delete(key);
      }
    }
    for (const key of keys) {
      if (waiting.current.has(key)) continue;
      const entry = {
        ready: false,
        timer: setTimeout(() => {
          entry.ready = true;
          redraw((revision) => revision + 1);
        }, approvalDisplayDelay),
      };
      waiting.current.set(key, entry);
    }
  }, [pending]);
  useEffect(
    () => () => {
      for (const entry of waiting.current.values()) clearTimeout(entry.timer);
      waiting.current.clear();
    },
    [],
  );
  return pending.filter(
    (event) =>
      isQuestionEvent(event) || waiting.current.get(identity(event))?.ready,
  );
}
