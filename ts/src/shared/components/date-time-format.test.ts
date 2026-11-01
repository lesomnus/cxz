import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  dateTimeDate,
  formatOffsetDateTime,
  formatRelativeDateTime,
} from "./date-time-format";

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

describe("resource date label details", () => {
  const now = Date.UTC(2026, 10, 1, 12);

  it.each([
    [0, "now"],
    [59_000, "now"],
    [60_000, "1 minute ago"],
    [3_600_000, "1 hour ago"],
    [86_400_000, "yesterday"],
    [7 * 86_400_000, "last week"],
    [30 * 86_400_000, "last month"],
    [365 * 86_400_000, "last year"],
    [-120_000, "in 2 minutes"],
  ])("humanizes an age of %s ms", (age, expected) => {
    expect(formatRelativeDateTime(new Date(now - age), "en", now)).toBe(
      expected,
    );
  });

  it("uses the selected language", () => {
    expect(formatRelativeDateTime(new Date(now - 120_000), "ko", now)).toBe(
      "2분 전",
    );
  });

  it("accepts protobuf timestamps and millisecond values, including the epoch", () => {
    expect(
      dateTimeDate(
        create(TimestampSchema, { seconds: 1n, nanos: 500_000_000 }),
      )?.getTime(),
    ).toBe(1500);
    expect(dateTimeDate(0n)?.getTime()).toBe(0);
    expect(dateTimeDate(1500)?.getTime()).toBe(1500);
  });

  it("omits missing and invalid dates", () => {
    expect(dateTimeDate()).toBeUndefined();
    expect(dateTimeDate(NaN)).toBeUndefined();
    expect(dateTimeDate(Infinity)).toBeUndefined();
    expect(dateTimeDate(10n ** 30n)).toBeUndefined();
  });
});
