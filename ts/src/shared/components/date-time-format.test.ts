import { afterEach, describe, expect, it, vi } from "vitest";
import { formatOffsetDateTime } from "./date-time-format";

afterEach(() => vi.restoreAllMocks());

describe("local timestamps with numeric offsets", () => {
  it.each([
    [0, "+00:00"],
    [-540, "+09:00"],
    [-330, "+05:30"],
    [-345, "+05:45"],
    [210, "-03:30"],
    [300, "-05:00"],
  ])(
    "formats timezone offset %s without losing fractional hours",
    (offset, suffix) => {
      const date = new Date(2026, 9, 20, 2, 38, 19);
      vi.spyOn(date, "getTimezoneOffset").mockReturnValue(offset);
      expect(formatOffsetDateTime(date)).toBe(`2026-10-20T02:38:19${suffix}`);
    },
  );

  it("pads all local calendar and clock fields", () => {
    const date = new Date(2026, 0, 2, 3, 4, 5);
    vi.spyOn(date, "getTimezoneOffset").mockReturnValue(0);
    expect(formatOffsetDateTime(date)).toBe("2026-01-02T03:04:05+00:00");
  });
});
