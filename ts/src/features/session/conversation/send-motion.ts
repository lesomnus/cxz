import { useEffect, useLayoutEffect, useRef, type RefObject } from "react";
import type { SessionEvent } from "../../../../gen/cxz/session_pb";

type SendMotion = {
  text: string;
  draft: string;
  after: bigint;
  source: HTMLElement;
  snapshot: HTMLElement;
  observer: MutationObserver;
  timeout: number;
  raf: number;
  ready: boolean;
  row?: HTMLElement;
  exit?: Animation;
  arrival?: Animation;
  started: boolean;
};

// Decorative copies and transforms never change the journal or measured row heights.
export function useSendMotion({
  source,
  pane,
  events,
  draft,
}: {
  source: RefObject<HTMLDivElement | null>;
  pane: RefObject<HTMLDivElement | null>;
  events: SessionEvent[];
  draft: string;
}) {
  const active = useRef<SendMotion | undefined>(undefined);
  const latest = useRef({ events, draft });
  latest.current = { events, draft };

  function clearRow(motion: SendMotion) {
    motion.arrival?.cancel();
    motion.row?.removeAttribute("data-send-arrival");
    motion.row = undefined;
    motion.arrival = undefined;
    motion.started = false;
  }
  function finish(motion = active.current) {
    if (!motion || active.current !== motion) return;
    active.current = undefined;
    clearTimeout(motion.timeout);
    cancelAnimationFrame(motion.raf);
    motion.observer.disconnect();
    motion.exit?.cancel();
    motion.snapshot.remove();
    motion.source.removeAttribute("data-send-exiting");
    clearRow(motion);
  }
  function timing(node: HTMLElement, token: string) {
    const value = getComputedStyle(node).getPropertyValue(token).trim();
    // Production CSS can normalize milliseconds to seconds; WAAPI uses ms.
    const number = parseFloat(value);
    return value.endsWith("s") && !value.endsWith("ms")
      ? number * 1000
      : number;
  }
  function locate() {
    const motion = active.current;
    if (!motion) return;
    const event = latest.current.events.find(
      (event) =>
        event.seq > motion.after &&
        event.kind === "input" &&
        event.text === motion.text,
    );
    if (!event) return;
    const row = pane.current?.querySelector<HTMLElement>(
      `article.input[data-seq="${event.seq}"]`,
    );
    const box = row?.querySelector<HTMLElement>(".input-box");
    if (!row || !box) return;
    if (motion.row !== row) {
      clearRow(motion);
      motion.row = row;
      row.dataset.sendArrival = "true";
      const travel = timing(box, "--send-enter-travel");
      const scale = timing(box, "--send-content-scale");
      motion.arrival = box.animate(
        [
          { transform: `translateY(${travel}px) scale(${scale})`, opacity: 0 },
          { transform: "translateY(0) scale(1)", opacity: 1 },
        ],
        {
          duration: timing(box, "--send-enter-duration"),
          easing: "cubic-bezier(0.2, 0.7, 0.2, 1)",
          fill: "both",
        },
      );
      motion.arrival.pause();
      motion.arrival.currentTime = 0;
    }
    if (motion.ready && !motion.started) {
      motion.started = true;
      motion.arrival!.play();
      void motion.arrival!.finished.then(
        () => finish(motion),
        () => {},
      );
    }
  }
  useLayoutEffect(() => {
    locate();
    const motion = active.current;
    // Preserve edits made while the request or its departure animation is pending.
    if (motion?.exit && draft !== motion.draft) motion.exit.finish();
  }, [events, draft]);
  useEffect(() => {
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    const changed = () => {
      if (reduced.matches) finish();
    };
    reduced.addEventListener("change", changed);
    return () => {
      reduced.removeEventListener("change", changed);
      finish();
    };
  }, []);

  function prepare(text: string, after: bigint) {
    finish();
    const node = source.current;
    const editor = node?.querySelector<HTMLElement>(".composer-editor");
    if (
      !node ||
      !editor ||
      !pane.current ||
      typeof editor.animate !== "function" ||
      matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;
    const snapshot = editor.cloneNode(true) as HTMLElement;
    // Only submitted source text departs; unaccepted command hints are decoration.
    snapshot.removeAttribute("data-command-open");
    // Plain drafts paint natively while editing; departure uses their mirror.
    snapshot.dataset.decorated = "true";
    snapshot.querySelector(".command-suggestions")?.remove();
    snapshot.classList.add("composer-send-ghost");
    snapshot.setAttribute("aria-hidden", "true");
    snapshot.inert = true;
    snapshot.style.height = `${editor.clientHeight}px`;
    snapshot.querySelector("textarea")?.remove();
    snapshot
      .querySelectorAll("[id]")
      .forEach((node) => node.removeAttribute("id"));
    const gutter = snapshot.querySelector<HTMLElement>(".editor-gutter");
    if (gutter) gutter.style.visibility = "hidden";
    const motion: SendMotion = {
      text,
      draft: latest.current.draft,
      after,
      source: node,
      snapshot,
      observer: new MutationObserver(locate),
      timeout: 0,
      raf: 0,
      ready: false,
      started: false,
    };
    active.current = motion;
    // Virtual rows can mount after the live event or after returning to Latest.
    motion.observer.observe(pane.current, { childList: true, subtree: true });
    motion.timeout = window.setTimeout(() => finish(motion), 10_000);
    return motion;
  }
  async function depart(motion: SendMotion | undefined) {
    if (!motion || active.current !== motion) return;
    if (latest.current.draft !== motion.draft) return;
    const { source: node, snapshot } = motion;
    node.append(snapshot);
    node.dataset.sendExiting = "true";
    const mirror = snapshot.querySelector<HTMLElement>(".editor-mirror")!;
    motion.exit = mirror.animate(
      [
        { transform: "translateY(0) scale(1)", opacity: 1 },
        {
          transform: `translateY(${-timing(node, "--send-exit-travel")}px) scale(${timing(node, "--send-content-scale")})`,
          opacity: 0,
        },
      ],
      {
        duration: timing(node, "--send-exit-duration"),
        easing: "cubic-bezier(0.5, 0, 0.8, 0.4)",
        fill: "both",
      },
    );
    await motion.exit.finished.catch(() => {});
  }
  function enter(motion: SendMotion | undefined) {
    if (!motion || active.current !== motion) return;
    motion.snapshot.remove();
    motion.source.removeAttribute("data-send-exiting");
    // Let clearing/contracting the composer and virtual-row measurement settle.
    motion.raf = requestAnimationFrame(() => {
      motion.raf = requestAnimationFrame(() => {
        if (active.current !== motion) return;
        motion.ready = true;
        locate();
      });
    });
  }
  return { prepare, depart, enter, cancel: finish };
}
