import { afterEach, expect, it, vi } from "vitest";
import { logPeriodBounds } from "./log-period";

afterEach(() => vi.unstubAllEnvs());

it("supports open-ended periods and includes the final local day", () => {
  vi.stubEnv("TZ", "Asia/Seoul");
  expect(logPeriodBounds("", "")).toEqual({
    since: undefined,
    until: undefined,
  });
  expect(logPeriodBounds("2026-11-13", "")).toEqual({
    since: Date.parse("2026-11-13T00:00:00+09:00"),
    until: undefined,
  });
  expect(logPeriodBounds("", "2026-11-13")).toEqual({
    since: undefined,
    until: Date.parse("2026-11-14T00:00:00+09:00"),
  });
});

it.each([
  ["2026-03-08", "2026-03-08T00:00:00-05:00", "2026-03-09T00:00:00-04:00", 23],
  ["2026-11-01", "2026-11-01T00:00:00-04:00", "2026-11-02T00:00:00-05:00", 25],
])(
  "uses calendar midnight across daylight saving on %s",
  (date, start, end, hours) => {
    vi.stubEnv("TZ", "America/New_York");
    const bounds = logPeriodBounds(date, date);
    expect(bounds).toEqual({
      since: Date.parse(start),
      until: Date.parse(end),
    });
    expect(bounds.until! - bounds.since!).toBe(hours * 60 * 60 * 1000);
  },
);
