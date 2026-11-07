import { expect, it } from "vitest";
import { TaskPageWheel } from "./task-page-wheel";

it("accumulates small trackpad deltas and consumes the rest of a gesture", () => {
  const wheel = new TaskPageWheel();
  expect(wheel.step(20, 0, 380)).toBe(0);
  expect(wheel.step(20, 20, 380)).toBe(0);
  expect(wheel.step(24, 40, 380)).toBe(1);
  for (let time = 60; time < 1000; time += 20)
    expect(wheel.step(100, time, 380)).toBe(0);
  expect(wheel.step(-100, 1500, 380)).toBe(-1);
});

it("resets a reversed accumulation and waits for an in-progress transition", () => {
  const wheel = new TaskPageWheel();
  expect(wheel.step(40, 0, 380)).toBe(0);
  expect(wheel.step(-40, 20, 380)).toBe(0);
  expect(wheel.step(-24, 40, 380)).toBe(-1);
  expect(wheel.step(100, 240, 380)).toBe(0);
  expect(wheel.step(100, 800, 380)).toBe(1);
});

it("accepts a new gesture without an animation lock in reduced motion", () => {
  const wheel = new TaskPageWheel();
  expect(wheel.step(NaN, 0, 0)).toBe(0);
  expect(wheel.step(0, 0, 0)).toBe(0);
  expect(wheel.step(100, 0, 0)).toBe(1);
  expect(wheel.step(-100, 200, 0)).toBe(-1);
});
