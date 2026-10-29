import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import type { SecretPreview } from "./secret-preview";
import styles from "./paste-preview.module.css";

export function SecretPreviewBody({ preview }: { preview: SecretPreview }) {
  const snapshot = useSyncExternalStore(preview.subscribe, preview.getSnapshot);
  const node = useRef<HTMLPreElement>(null);
  const [revealed, reveal] = useState(false);
  useEffect(preview.connect, [preview]);
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
  return (
    <pre
      ref={node}
      className={styles.secret}
      data-revealed={revealed}
      aria-label="Secret value"
    >
      {snapshot.value}
    </pre>
  );
}
