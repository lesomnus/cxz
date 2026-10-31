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
};

export const approvalBacks = (requests: readonly SessionEvent[]) =>
  requests.slice(1, 3);

export function useApprovalPresence(
  requests: readonly SessionEvent[],
  open: boolean,
  front: RefObject<HTMLDivElement | null>,
) {
  const visible = open ? requests : empty;
  const current = visible[0];
  const [leaving, setLeaving] = useState<Departure | null>(null);
  const [entryDepth, setEntryDepth] = useState(0);
  const [promotionKey, setPromotionKey] = useState("");
  const previous = useRef({ requests: empty, height: 0 });

  useLayoutEffect(() => {
    const before = previous.current;
    const from = before.requests[0];
    if (approvalKey(from) !== approvalKey(current)) {
      const depth = before.requests.findIndex(
        (event) => approvalKey(event) === approvalKey(current),
      );
      setEntryDepth(Math.max(0, Math.min(2, depth)));
      setPromotionKey(depth > 0 ? approvalKey(current) : "");
      setLeaving((old) => {
        if (window.matchMedia("(prefers-reduced-motion: reduce)").matches)
          return null;
        if (from)
          return {
            event: from,
            height: before.height,
            backs: current ? empty : approvalBacks(before.requests),
          };
        return approvalKey(old?.event) === approvalKey(current) ? null : old;
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
  const finishPromotion = (key: string) =>
    setPromotionKey((old) => (old === key ? "" : old));
  useEffect(() => {
    if (!promotionKey) return;
    const timer = window.setTimeout(() => finishPromotion(promotionKey), 1000);
    return () => window.clearTimeout(timer);
  }, [promotionKey]);
  useEffect(() => {
    if (!leaving) return;
    // Also remove inert departures if an animation is canceled or motion changes.
    const timer = window.setTimeout(
      () => finish(approvalKey(leaving.event)),
      1000,
    );
    return () => window.clearTimeout(timer);
  }, [leaving]);

  return {
    current,
    visible,
    leaving,
    entryDepth,
    finish,
    promotionKey,
    finishPromotion,
  };
}
