import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";

export const approvalKey = (event?: SessionEvent) =>
  event ? `${event.runId}:${event.requestId}:${event.seq}` : "";

const empty: readonly SessionEvent[] = [];
type Departure = {
  event: SessionEvent;
  height: number;
  backs: readonly SessionEvent[];
  frontDone?: boolean;
};

export const approvalBacks = (requests: readonly SessionEvent[]) =>
  requests.length > 3
    ? [requests[1], requests[requests.length - 1]]
    : requests.slice(1, 3);

export function useApprovalPresence(
  requests: readonly SessionEvent[],
  open: boolean,
  front: RefObject<HTMLDivElement | null>,
) {
  const visible = open ? requests : empty;
  const current = visible[0];
  const [leaving, setLeaving] = useState<Departure | null>(null);
  const [entryDepth, setEntryDepth] = useState(0);
  const previous = useRef({ requests: empty, height: 0 });

  useLayoutEffect(() => {
    const before = previous.current;
    const from = before.requests[0];
    if (approvalKey(from) !== approvalKey(current)) {
      const depth = before.requests.findIndex(
        (event) => approvalKey(event) === approvalKey(current),
      );
      setEntryDepth(Math.max(0, Math.min(2, depth)));
      setLeaving((old) => {
        if (window.matchMedia("(prefers-reduced-motion: reduce)").matches)
          return null;
        if (from)
          return {
            event: from,
            height: before.height,
            backs: current ? empty : approvalBacks(before.requests),
          };
        return approvalKey(old?.event) === approvalKey(current) ||
          (current && old?.frontDone)
          ? null
          : old;
      });
    }
    const snapshot = {
      requests: [...visible],
      height: front.current
        ? parseFloat(getComputedStyle(front.current).height)
        : 0,
    };
    previous.current = snapshot;
    const node = front.current;
    if (!node) return;
    const observer = new ResizeObserver(() => {
      snapshot.height = parseFloat(getComputedStyle(node).height);
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [visible, current, front]);

  const finish = (key: string) =>
    setLeaving((old) => (approvalKey(old?.event) === key ? null : old));
  const finishFront = (key: string) =>
    setLeaving((old) =>
      approvalKey(old?.event) === key && old
        ? { ...old, frontDone: true }
        : old,
    );
  useEffect(() => {
    if (!leaving) return;
    // Also remove inert departures if an animation is canceled or motion changes.
    const timer = window.setTimeout(
      () => finish(approvalKey(leaving.event)),
      1000,
    );
    return () => window.clearTimeout(timer);
  }, [leaving]);

  return { current, visible, leaving, entryDepth, finish, finishFront };
}
