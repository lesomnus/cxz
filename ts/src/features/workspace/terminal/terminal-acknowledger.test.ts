import { afterEach, expect, it, vi } from "vitest";
import { createTerminalAcknowledger } from "./terminal-acknowledger.ts";

afterEach(() => vi.useRealTimers());

it("combines parsed output and flushes a partial tail without waiting for more data", () => {
  vi.useFakeTimers();
  const send = vi.fn();
  const ack = createTerminalAcknowledger(send);
  ack.parsed(100);
  ack.parsed(200);
  expect(send).not.toHaveBeenCalled();
  vi.advanceTimersByTime(5);
  expect(send.mock.calls).toEqual([[300]]);
  ack.parsed(10);
  vi.advanceTimersByTime(5);
  expect(send.mock.calls).toEqual([[300], [10]]);
});

it("returns credit immediately for large output and cancels the old timer", () => {
  vi.useFakeTimers();
  const send = vi.fn();
  const ack = createTerminalAcknowledger(send);
  ack.parsed(100);
  ack.parsed(32768);
  expect(send.mock.calls).toEqual([[32868]]);
  vi.advanceTimersByTime(20);
  expect(send).toHaveBeenCalledTimes(1);
});

it("ignores delayed write callbacks and timers after the connection is disposed", () => {
  vi.useFakeTimers();
  const send = vi.fn();
  const ack = createTerminalAcknowledger(send);
  ack.parsed(123);
  ack.dispose();
  ack.parsed(32768);
  vi.runAllTimers();
  expect(send).not.toHaveBeenCalled();
});
