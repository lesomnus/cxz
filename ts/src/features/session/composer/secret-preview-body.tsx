import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import type { SecretPreview } from "./secret-preview";
import { usePreviewSurface } from "#src/features/session/cards/floating-card.tsx";
import styles from "./paste-preview.module.css";

export function SecretPreviewBody({ preview }: { preview: SecretPreview }) {
  const snapshot = useSyncExternalStore(preview.subscribe, preview.getSnapshot);
  const surface = usePreviewSurface();
  const node = useRef<HTMLElement | null>(null);
  const [revealed, reveal] = useState(false);
  useEffect(preview.connect, [preview]);
  useLayoutEffect(() => {
    if (snapshot.state !== "ready") return;
    const body = surface?.current?.querySelector<HTMLElement>(
      ":scope > .card-body",
    );
    if (!body) return;
    node.current = body;
    const label = body.getAttribute("aria-label");
    body.setAttribute("aria-label", "Secret value");
    body.classList.add(styles.secret);
    return () => {
      body.classList.remove(styles.secret);
      if (label === null) body.removeAttribute("aria-label");
      else body.setAttribute("aria-label", label);
      delete body.dataset.revealed;
      node.current = null;
    };
  }, [surface, snapshot.state]);
  useLayoutEffect(() => {
    if (node.current) node.current.dataset.revealed = String(revealed);
  }, [snapshot.state, revealed]);
  useEffect(() => {
    let start: { x: number; y: number } | undefined;
    let dragging = false;
    const selected = () => {
      const selection = window.getSelection();
      if (
        !node.current ||
        !selection ||
        selection.isCollapsed ||
        !selection.rangeCount
      )
        return false;
      return selection.getRangeAt(0).intersectsNode(node.current);
    };
    const changed = () =>
      reveal(
        document.hasFocus() &&
          document.visibilityState === "visible" &&
          (dragging || selected()),
      );
    const down = (event: PointerEvent) => {
      if (event.button !== 0 || event.pointerType === "touch") return;
      start = { x: event.clientX, y: event.clientY };
    };
    const move = (event: PointerEvent) => {
      if (
        start &&
        event.buttons === 1 &&
        Math.hypot(event.clientX - start.x, event.clientY - start.y) >= 3
      ) {
        dragging = true;
        reveal(true);
      }
    };
    const end = () => {
      start = undefined;
      dragging = false;
      changed();
    };
    const conceal = () => {
      start = undefined;
      dragging = false;
      reveal(false);
    };
    const element = node.current;
    element?.addEventListener("pointerdown", down);
    document.addEventListener("pointermove", move);
    document.addEventListener("pointerup", end);
    document.addEventListener("pointercancel", conceal);
    document.addEventListener("selectionchange", changed);
    window.addEventListener("blur", conceal);
    document.addEventListener("visibilitychange", conceal);
    return () => {
      element?.removeEventListener("pointerdown", down);
      document.removeEventListener("pointermove", move);
      document.removeEventListener("pointerup", end);
      document.removeEventListener("pointercancel", conceal);
      document.removeEventListener("selectionchange", changed);
      window.removeEventListener("blur", conceal);
      document.removeEventListener("visibilitychange", conceal);
    };
  }, [snapshot.state]);
  if (snapshot.state === "error") return <p role="alert">{snapshot.error}</p>;
  if (snapshot.state !== "ready") return <p role="status">Loading secret…</p>;
  return snapshot.value;
}
