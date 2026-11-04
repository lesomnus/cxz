import { useMemo, useRef, type ComponentProps } from "react";
import {
  AnimatedText,
  CompactNumber,
  DurationDisplay,
  type TextMotionOptions,
} from "@lesomnus/nox";
import styles from "./animated-value.module.css";

// Only value changes start motion. Visibility is sampled against the window
// and every clipping ancestor; scrolling an already committed value into view
// does not schedule an entrance.
function visibleOrigin(element: HTMLElement) {
  if (document.visibilityState !== "visible") return;
  const bounds = element.getBoundingClientRect();
  if (!bounds.width || !bounds.height) return;
  let left = 0,
    top = 0;
  let right = document.documentElement.clientWidth;
  let bottom = document.documentElement.clientHeight;
  for (
    let parent = element.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    const css = getComputedStyle(parent);
    if (css.visibility === "hidden" || css.display === "none") return;
    const clip = parent.getBoundingClientRect();
    if (/auto|scroll|hidden|clip/.test(css.overflowX)) {
      left = Math.max(left, clip.left + parent.clientLeft);
      right = Math.min(
        right,
        clip.left + parent.clientLeft + parent.clientWidth,
      );
    }
    if (/auto|scroll|hidden|clip/.test(css.overflowY)) {
      top = Math.max(top, clip.top + parent.clientTop);
      bottom = Math.min(
        bottom,
        clip.top + parent.clientTop + parent.clientHeight,
      );
    }
  }
  if (
    bounds.right <= left ||
    bounds.left >= right ||
    bounds.bottom <= top ||
    bounds.top >= bottom
  )
    return;
  return Math.round(
    Math.max(0, bounds.top) * 0.7 + Math.max(0, bounds.left) * 0.08,
  );
}

export function useValueMotion(enabled = true): TextMotionOptions {
  const origin = useRef(0);
  return useMemo(
    () => ({
      motion: enabled ? "slide" : "none",
      animateInitial: enabled,
      duration: 400,
      shouldAnimate: (element: HTMLElement) => {
        const delay = visibleOrigin(element);
        if (delay === undefined) return false;
        origin.current = delay;
        return true;
      },
      // The first changed character anchors the value's spatial start; the rest
      // shuffle inside that value without moving the start of the screen wave.
      characterDelay: ({ changedIndex }) =>
        origin.current +
        (changedIndex ? 1 + Math.floor(Math.random() * 90) : 0),
    }),
    [enabled],
  );
}

export function AnimatedValue({
  enabled = true,
  className = "",
  ...props
}: ComponentProps<typeof AnimatedText> & { enabled?: boolean }) {
  return (
    <AnimatedText
      {...props}
      {...useValueMotion(enabled)}
      className={`${styles.value} ${className}`}
    />
  );
}

export function AnimatedCompactNumber(
  props: ComponentProps<typeof CompactNumber>,
) {
  return (
    <CompactNumber
      {...props}
      {...useValueMotion(Number.isFinite(props.value))}
    />
  );
}

export function AnimatedDurationDisplay(
  props: ComponentProps<typeof DurationDisplay>,
) {
  return (
    <DurationDisplay
      {...props}
      {...useValueMotion(props.durationMs !== undefined)}
    />
  );
}
