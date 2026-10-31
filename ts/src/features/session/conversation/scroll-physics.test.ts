import { describe, expect, it } from "vitest";
import {
  edgePull,
  dragPullDistance,
  scrollRange,
  markerRangeStart,
  SCROLL_WINDOW_SCREENS,
  resumeRange,
  smoothStep,
} from "./scroll-physics";
import { quotaDots } from "#src/features/session/model/session-info.ts";

describe("bounded scroll handle", () => {
  it("starts a continuous symmetric pull in the outer quarters before either end", () => {
    for (const travel of [200, 400, 800]) {
      for (const fraction of [0.25, 0.5, 0.75])
        expect(dragPullDistance(travel * fraction, travel)).toBe(0);
      const light = dragPullDistance(travel * 0.2, travel);
      const strong = dragPullDistance(travel * 0.1, travel);
      expect(light).toBeLessThan(0);
      expect(strong).toBeLessThan(light);
      expect(dragPullDistance(travel * 0.8, travel)).toBeCloseTo(-light);
      expect(dragPullDistance(travel * 0.9, travel)).toBeCloseTo(-strong);
      expect(edgePull(strong, 800).speed).toBeLessThan(
        edgePull(light, 800).speed,
      );
      expect(
        Math.abs(
          edgePull(dragPullDistance(travel * 0.25 - 0.01, travel), 800).speed,
        ),
      ).toBeLessThan(0.1);
      expect(dragPullDistance(-50, travel)).toBe(-50 - travel * 0.25);
    }
  });
  it("preserves fractional handle motion instead of snapping small updates", () => {
    const next = smoothStep(0, 0.8, 16, 28, 0.05);
    expect(next).toBeGreaterThan(0);
    expect(next).toBeLessThan(0.8);
    expect(smoothStep(next, 0.8, 200, 28, 0.05)).toBe(0.8);
  });
  it("keeps its range bounded as history grows and preserves a usable local range", () => {
    const before = scrollRange(50000, 100000, 800);
    const after = scrollRange(50100, 1000000, 800, before);
    expect(after).toEqual(before);
    expect(after.span).toBe(800 * SCROLL_WINDOW_SCREENS);
    expect(after.start).toBeLessThan(50100);
    expect(after.start + after.span).toBeGreaterThan(50100);
    expect(scrollRange(0, 100000, 800).start).toBe(0);
    const bottom = scrollRange(100000, 100000, 800);
    expect(bottom.start + bottom.span).toBe(100000);
  });
  it("moves the handle less per pixel while accelerating scroll in either direction", () => {
    const light = edgePull(40, 800),
      medium = edgePull(80, 800),
      strong = edgePull(120, 800);
    expect(strong.stretch - medium.stretch).toBeLessThan(
      medium.stretch - light.stretch,
    );
    expect(strong.speed).toBeGreaterThan(medium.speed);
    expect(medium.speed).toBeGreaterThan(light.speed);
    expect(edgePull(-120, 800)).toEqual({
      stretch: -strong.stretch,
      speed: -strong.speed,
    });
    expect(edgePull(0, 800)).toEqual({ stretch: 0, speed: 0 });
    expect(edgePull(10000, 800).stretch).toBeLessThanOrEqual(20);
    expect(edgePull(10000, 800).speed).toBeLessThanOrEqual(800 * 16);
  });
});

it("returns from either elastic edge without jumping back to the original window", () => {
  for (const fraction of [0.01, 0.99]) {
    const range = resumeRange(42000, fraction, 9600);
    expect(range.start + fraction * range.span).toBe(42000);
    expect(range.start + (fraction + 0.005) * range.span).toBeCloseTo(42048);
  }
});
it("smooths quickly without overshoot and lands exactly at the target in either direction", () => {
  for (const target of [600, -600]) {
    let top = smoothStep(0, target, 16);
    expect(Math.abs(top)).toBeGreaterThan(250);
    expect(Math.abs(top)).toBeLessThan(600);
    for (let frame = 0; frame < 12; frame++) {
      const next = smoothStep(top, target, 16);
      expect(Math.abs(next - target)).toBeLessThanOrEqual(
        Math.abs(top - target),
      );
      top = next;
    }
    expect(top).toBe(target);
  }
});
it("uses the TUI quota cells, including its visible empty baseline", () => {
  expect(quotaDots(0)).toBe("⣀⣀⣀⣀⣀⣀⣀⣀");
  expect(quotaDots(70)).toBe("⣿⣿⣿⣿⣿⣤⣀⣀");
  expect(quotaDots(120)).toBe("⣿⣿⣿⣿⣿⣿⣿⣿");
  expect(quotaDots(undefined)).toBe("—");
});

it("advances prompt mapping only for content passing a pinned drag edge", () => {
  const range = { start: 1000, span: 6000 };
  expect(markerRangeStart(3000, range)).toBe(1000);
  expect(markerRangeStart(800, range)).toBe(800);
  expect(markerRangeStart(100, range)).toBe(100);
  expect(markerRangeStart(7200, range)).toBe(1200);
  expect(markerRangeStart(8000, range)).toBe(2000);
  // Prepending history shifts both the transcript and mapping coordinates equally.
  expect(markerRangeStart(8200, { start: 2000, span: 6000 })).toBe(2200);
});

it("keeps page and height corrections from reversing a moving window", () => {
  const range = { start: 20000, span: 9600 };
  // Coordinate shifts are already applied to both the viewport and its range.
  expect(scrollRange(29500, 30000, 800, range, 29500)).toEqual(range);
  // A reduced cache tail cannot pull the mapping backwards while moving down.
  expect(
    scrollRange(29600, 30000, 800, range, 29500).start,
  ).toBeGreaterThanOrEqual(range.start);
  const shifted = { start: -4000, span: 9600 };
  // Prefix eviction can place the local mapping before the cached first row.
  expect(
    scrollRange(5000, 40000, 800, shifted, 5100).start,
  ).toBeLessThanOrEqual(shifted.start);
});
