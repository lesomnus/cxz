import { expect, it } from "vitest";
import {
  SEND_SPRING_DURATION,
  sendSpringState,
  sendSpringFrames,
} from "./send-spring";
it("starts at rest, accelerates under tension and settles near the target", () => {
  expect(sendSpringState(0, 8, 0, 0)).toEqual({ position: 0, velocity: 0 });
  const first = sendSpringState(0, 8, 0, 50).position;
  const second = sendSpringState(0, 8, 0, 100).position;
  expect(first).toBeGreaterThan(0);
  expect(second - first).toBeGreaterThan(first);
  expect(sendSpringState(0, 8, 0, SEND_SPRING_DURATION).position).toBeCloseTo(
    8,
    1,
  );
  expect(sendSpringFrames(0, 8).at(-1)?.transform).toBe(
    "translateY(8px) scale(1)",
  );
});
it("returns from the current displacement with continuous initial velocity", () => {
  const initial = sendSpringState(0, 8, 0, 100);
  const recoil = sendSpringState(initial.position, 0, initial.velocity, 0);
  expect(recoil.position).toBeCloseTo(initial.position);
  expect(recoil.velocity).toBeCloseTo(initial.velocity);
  expect(
    Math.abs(
      sendSpringState(
        initial.position,
        0,
        initial.velocity,
        SEND_SPRING_DURATION,
      ).position,
    ),
  ).toBeLessThan(0.1);
});
