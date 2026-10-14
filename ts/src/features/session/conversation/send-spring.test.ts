import { expect, it } from "vitest";
import {
  SEND_SPRING_DURATION,
  SEND_PULL_DURATION,
  sendPullState,
  sendPullFrames,
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

it("keeps pulling slowly during a long request without exceeding the limit", () => {
  expect(sendPullState(0)).toEqual({ position: 0, velocity: 0 });
  expect(sendPullState(500).position).toBeGreaterThan(2.8);
  expect(sendPullState(500).position).toBeLessThan(3.2);
  let previous = 0;
  for (let ms = 0; ms <= SEND_PULL_DURATION; ms += 50) {
    const { position, velocity } = sendPullState(ms);
    expect(position).toBeGreaterThanOrEqual(previous);
    expect(position).toBeLessThanOrEqual(8);
    expect(velocity).toBeGreaterThanOrEqual(0);
    previous = position;
  }
  expect(sendPullState(3000).position).toBeGreaterThan(5);
  expect(sendPullState(8000).position).toBeGreaterThan(7);
  expect(sendPullState(8000).velocity).toBeLessThan(
    sendPullState(3000).velocity,
  );
  expect(sendPullState(60000)).toEqual({ position: 8, velocity: 0 });
  expect(sendPullFrames().at(-1)).toEqual({
    offset: 1,
    transform: "translateY(8px) scale(1)",
    opacity: 1,
  });
});
