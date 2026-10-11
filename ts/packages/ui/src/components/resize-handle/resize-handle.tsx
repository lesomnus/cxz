import { useRef } from "react";
import styles from "./resize-handle.module.css";

/** Resize one neighboring pane without changing or remounting its content. */
export function ResizeHandle({
  axis = "horizontal",
  value,
  min,
  max,
  onChange,
  onReset,
  label = "Resize panes",
  className = "",
}: {
  axis?: "horizontal" | "vertical";
  value: number;
  min: number;
  max: number;
  onChange: (value: number) => void;
  onReset?: () => void;
  label?: string;
  className?: string;
}) {
  const origin = useRef({ position: 0, value: 0 });
  const horizontal = axis === "horizontal";
  const change = (next: number) => onChange(Math.max(min, Math.min(max, next)));
  return (
    <div
      className={`${styles.handle} ${className}`}
      data-axis={axis}
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation={horizontal ? "vertical" : "horizontal"}
      aria-valuemin={Math.round(min)}
      aria-valuemax={Math.round(max)}
      aria-valuenow={Math.round(value)}
      onDoubleClick={onReset}
      onPointerDown={(event) => {
        if (event.button !== 0) return;
        event.preventDefault();
        event.currentTarget.focus({ preventScroll: true });
        origin.current = {
          position: horizontal ? event.clientX : event.clientY,
          value,
        };
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={(event) => {
        if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
        change(
          origin.current.value +
            (horizontal ? event.clientX : event.clientY) -
            origin.current.position,
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
        const step = event.shiftKey ? 48 : 16;
        change(
          event.key === "Home"
            ? min
            : event.key === "End"
              ? max
              : value + (event.key === previous ? -step : step),
        );
      }}
    />
  );
}
