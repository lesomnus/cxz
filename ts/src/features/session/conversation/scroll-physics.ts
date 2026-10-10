import { clamp } from "../../../shared/lib/math";
export { clamp } from "../../../shared/lib/math";
export const SCROLL_WINDOW_SCREENS = 12;
export const ELASTIC_RESERVE = 20;
export type ScrollRange = { start: number; span: number };

// Heights round to integers while scrollTop stays fractional. Zoom can leave
// more than a pixel of reported gap even after the browser clamps to its end.
export function atScrollBottom(
  pane: Pick<HTMLElement, "scrollHeight" | "scrollTop" | "clientHeight">,
) {
  return pane.scrollHeight - pane.scrollTop - pane.clientHeight <= 2;
}

// The handle maps a local window, so adding older pages cannot shrink it forever.
export function scrollRange(
  top: number,
  max: number,
  viewport: number,
  previous?: ScrollRange,
  previousTop?: number,
): ScrollRange {
  const span = Math.min(max, viewport * SCROLL_WINDOW_SCREENS);
  // Page insertion/eviction and measured row heights change coordinates, not
  // the reading position. Keep their coordinate-adjusted mapping in place.
  if (previous?.span === span && top === previousTop) return previous;
  if (
    previous &&
    previous.span === span &&
    top >= previous.start &&
    top <= previous.start + span &&
    (top >= previous.start + span * 0.2 || previous.start === 0) &&
    (top <= previous.start + span * 0.8 || previous.start + span >= max)
  )
    return previous;
  let start = clamp(top - span / 2, 0, Math.max(0, max - span));
  if (previous?.span === span && previousTop !== undefined) {
    // A changing cache boundary must never rebase against the user's movement.
    start =
      top > previousTop
        ? Math.max(previous.start, start)
        : Math.min(previous.start, start);
  }
  return { start, span };
}

// Keep the handle pinned to its dragged edge while prompt ticks advance with
// content passing that edge. Inside the mapped range they share the same phase.
export function markerRangeStart(top: number, range: ScrollRange) {
  return range.start + top - clamp(top, range.start, range.start + range.span);
}

// A saturating spring moves less with each extra pixel of pull, while velocity
// increases. Both directions share the same curve and stop at actual history edges.
export function edgePull(distance: number, viewport: number) {
  const pull = Math.max(0, Math.abs(distance));
  return {
    stretch: Math.sign(distance) * ELASTIC_RESERVE * (1 - Math.exp(-pull / 80)),
    speed:
      Math.sign(distance) *
      viewport *
      Math.min(16, Math.pow(pull / 60, 1.4) * 2),
  };
}

// Re-enter the rail at the current reading position, rather than the position
// from before accelerated scrolling crossed the local window's edge.
export function resumeRange(
  top: number,
  fraction: number,
  span: number,
): ScrollRange {
  return { start: top - fraction * span, span };
}

// Short, non-overshooting wheel response. Finish exactly on the requested pixel.
export function smoothStep(
  top: number,
  target: number,
  elapsed: number,
  response = 24,
) {
  const next = top + (target - top) * (1 - Math.exp(-elapsed / response));
  return Math.abs(target - next) < 1 ? target : next;
}
