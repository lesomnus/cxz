import { useRef, type ReactNode } from "react";
import styles from "./split-layout.module.css";

export type SplitNode =
  | { kind: "pane"; id: string }
  | {
      kind: "split";
      id: string;
      axis: "horizontal" | "vertical";
      ratio: number;
      first: SplitNode;
      second: SplitNode;
    };

export function setSplitRatio(
  node: SplitNode,
  id: string,
  ratio: number,
): SplitNode {
  if (node.kind === "pane") return node;
  if (node.id === id)
    return { ...node, ratio: Math.max(0.15, Math.min(0.85, ratio)) };
  return {
    ...node,
    first: setSplitRatio(node.first, id, ratio),
    second: setSplitRatio(node.second, id, ratio),
  };
}

/** A controlled, recursive split layout. Pane content and docking are owned by the caller. */
export function SplitLayout({
  node,
  renderPane,
  onResize,
  separatorLabel = "Resize panes",
}: {
  node: SplitNode;
  renderPane: (id: string) => ReactNode;
  onResize: (id: string, ratio: number) => void;
  separatorLabel?: string;
}) {
  const root = useRef<HTMLDivElement>(null);
  if (node.kind === "pane")
    return <div className={styles.pane}>{renderPane(node.id)}</div>;
  const horizontal = node.axis === "horizontal";
  return (
    <div ref={root} className={styles.split} data-axis={node.axis}>
      <div className={styles.part} style={{ flex: `${node.ratio} 1 0%` }}>
        <SplitLayout
          node={node.first}
          renderPane={renderPane}
          onResize={onResize}
          separatorLabel={separatorLabel}
        />
      </div>
      <div
        className={styles.separator}
        role="separator"
        tabIndex={0}
        aria-label={separatorLabel}
        aria-orientation={horizontal ? "vertical" : "horizontal"}
        aria-valuemin={15}
        aria-valuemax={85}
        aria-valuenow={Math.round(node.ratio * 100)}
        onDoubleClick={() => onResize(node.id, 0.5)}
        onPointerDown={(event) => {
          if (event.button !== 0) return;
          event.preventDefault();
          event.currentTarget.focus({ preventScroll: true });
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
          const bounds = root.current!.getBoundingClientRect();
          const size = horizontal ? bounds.width : bounds.height;
          if (size)
            onResize(
              node.id,
              (horizontal
                ? event.clientX - bounds.left
                : event.clientY - bounds.top) / size,
            );
        }}
        onPointerUp={(event) => {
          if (event.currentTarget.hasPointerCapture(event.pointerId))
            event.currentTarget.releasePointerCapture(event.pointerId);
        }}
        onKeyDown={(event) => {
          const previous = horizontal ? "ArrowLeft" : "ArrowUp";
          const next = horizontal ? "ArrowRight" : "ArrowDown";
          if (![previous, next, "Home", "End"].includes(event.key)) return;
          event.preventDefault();
          const step = event.shiftKey ? 0.1 : 0.02;
          onResize(
            node.id,
            event.key === "Home"
              ? 0.15
              : event.key === "End"
                ? 0.85
                : node.ratio + (event.key === previous ? -step : step),
          );
        }}
      />
      <div className={styles.part} style={{ flex: `${1 - node.ratio} 1 0%` }}>
        <SplitLayout
          node={node.second}
          renderPane={renderPane}
          onResize={onResize}
          separatorLabel={separatorLabel}
        />
      </div>
    </div>
  );
}
