import { useEffect, useLayoutEffect, useRef, type RefObject } from "react";
import type { RowLayout } from "./virtual-layout";

type Position = { node: HTMLElement; top: number };

// Move only the painted rows. The virtual heights, logical map and scroll target
// settle immediately; historical paging and deliberate navigation stay direct.
export function useTranscriptMotion({
  root,
  pane,
  follow,
  rows,
  start,
  end,
}: {
  root: RefObject<HTMLDivElement | null>;
  pane: RefObject<HTMLDivElement | null>;
  follow: RefObject<boolean>;
  rows: RowLayout[];
  start: number;
  end: number;
}) {
  const saved = useRef<
    | {
        positions: Map<string, Position>;
        scroll: number;
        width: number;
        tail?: bigint;
      }
    | undefined
  >(undefined);
  const moves = useRef(new Map<HTMLElement, Animation>());
  const arrivals = useRef(new Map<HTMLElement, number>());
  const pending = useRef(new Map<string, number>());
  const settling = useRef(0);

  function capture(tail = saved.current?.tail) {
    const el = pane.current!,
      column = root.current!;
    return {
      tail,
      scroll: el.scrollTop,
      width: column.clientWidth,
      positions: new Map(
        [...column.querySelectorAll<HTMLElement>("[data-row]")].map((node) => [
          node.dataset.row!,
          { node, top: column.offsetTop + node.offsetTop - el.scrollTop },
        ]),
      ),
    };
  }
  function stop() {
    for (const animation of moves.current.values()) animation.cancel();
    moves.current.clear();
    for (const [node, timeout] of arrivals.current) {
      clearTimeout(timeout);
      node.removeAttribute("data-live-arrival");
    }
    arrivals.current.clear();
    pending.current.clear();
    settling.current = 0;
  }
  useEffect(() => {
    const el = pane.current;
    if (!el || !root.current) return;
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    const navigate = (event?: Event) => {
      // The local Send handshake resumes following after its departure. It is
      // part of the same arrival, rather than a new reading gesture.
      if ((event as CustomEvent<string> | undefined)?.detail === "send") return;
      stop();
      saved.current = capture();
    };
    const scroll = () => {
      // Programmatic following already published this position in the layout
      // effect. Only a different position indicates a user's reading movement.
      if (el.scrollTop !== saved.current?.scroll) navigate();
    };
    const changed = () => {
      if (reduced.matches) navigate();
    };
    const navigationEvents = [
      "wheel",
      "touchstart",
      "pointerdown",
      "scroll-jump",
      "reading-move",
    ];
    for (const name of navigationEvents)
      el.addEventListener(name, navigate, { passive: true });
    el.addEventListener("scroll", scroll);
    reduced.addEventListener("change", changed);
    return () => {
      stop();
      for (const name of navigationEvents)
        el.removeEventListener(name, navigate);
      el.removeEventListener("scroll", scroll);
      reduced.removeEventListener("change", changed);
    };
  }, []);
  useLayoutEffect(() => {
    // Child layout effects run before the parent's pane ref is attached on the
    // first commit. The initial measurement supplies the baseline afterwards.
    if (!pane.current || !root.current) return;
    const old = saved.current;
    const tail = rows.at(-1)?.id;
    const sequence = tail ? BigInt(tail) : undefined;
    const high =
      sequence !== undefined && (old?.tail === undefined || sequence > old.tail)
        ? sequence
        : old?.tail;
    const next = capture(high);
    saved.current = next;
    if (
      !old ||
      old.tail === undefined ||
      !follow.current ||
      old.width !== next.width ||
      matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      stop();
      return;
    }
    const now = performance.now();
    const style = getComputedStyle(root.current!);
    const durationOf = (name: string) => {
      const token = style.getPropertyValue(name).trim();
      return parseFloat(token) * (token.endsWith("ms") ? 1 : 1000);
    };
    const duration = durationOf("--transcript-shift-duration");
    const entranceDuration = durationOf("--transcript-enter-duration");
    const easing = style.getPropertyValue("--transcript-motion-easing").trim();
    const settleDuration = Math.max(duration, entranceDuration);
    if (sequence !== undefined && sequence > old.tail) {
      settling.current = now + settleDuration;
      for (const row of rows)
        if (BigInt(row.id) > old.tail)
          pending.current.set(row.id, now + settleDuration);
    }
    if (now > settling.current) return;
    for (const [id, position] of next.positions) {
      const before = old.positions.get(id),
        node = position.node;
      if (before?.node === node) {
        const delta = before.top - position.top;
        if (Math.abs(delta) > 0.5) {
          const transform = getComputedStyle(node).transform;
          const offset =
            transform === "none" ? 0 : new DOMMatrix(transform).m42;
          moves.current.get(node)?.cancel();
          const animation = node.animate(
            [
              { transform: `translateY(${delta + offset}px)` },
              { transform: "translateY(0)" },
            ],
            { duration, easing },
          );
          moves.current.set(node, animation);
          void animation.finished.then(
            () => {
              if (moves.current.get(node) === animation)
                moves.current.delete(node);
            },
            () => {},
          );
        }
      }
      const deadline = pending.current.get(id);
      if (deadline !== undefined) {
        pending.current.delete(id);
        if (deadline > now) {
          node.dataset.liveArrival = "true";
          const timeout = window.setTimeout(() => {
            node.removeAttribute("data-live-arrival");
            arrivals.current.delete(node);
          }, entranceDuration);
          arrivals.current.set(node, timeout);
        }
      }
    }
    for (const [node, animation] of moves.current)
      if (!node.isConnected) {
        animation.cancel();
        moves.current.delete(node);
      }
  }, [rows, start, end]);
}
