import { describe, expect, it } from "vitest";
import { formatCompactNumber } from "@lesomnus/nox";

const text = (value: number) => {
  const result = formatCompactNumber(value);
  return result.leading + result.digits + result.unit;
};
describe("compact metric numbers", () => {
  it("preserves the requested numeric and unit cells across ranges", () => {
    for (const [value, expected] of [
      [0, "0000·"],
      [1, "0001·"],
      [999, "0999·"],
      [1000, "1.00K"],
      [1230, "1.23K"],
      [12_300, "12.3K"],
      [123_000, "0123K"],
      [1_230_000, "1.23M"],
      [999_400, "0999K"],
      [999_500, "1.00M"],
    ] as const)
      expect(text(value)).toBe(expected);
  });
  it("dims padding only, including the actual zero case", () => {
    expect(formatCompactNumber(0)).toEqual({
      leading: "000",
      digits: "0",
      unit: "·",
    });
    expect(formatCompactNumber(1)).toEqual({
      leading: "000",
      digits: "1",
      unit: "·",
    });
    expect(formatCompactNumber(1230).leading).toBe("");
  });
  it("uses a smaller unit when it preserves more of the original number", () => {
    expect(text(1_234_567)).toBe("1235K");
    expect(text(1_234_000)).toBe("1234K");
    expect(text(0.001234)).toBe("1234µ");
  });
  it("keeps ordinary counters integral without a unit cell", () => {
    for (const [value, expected] of [
      [1, "0001"],
      [999, "0999"],
      [1234, "1234"],
    ] as const) {
      const number = formatCompactNumber(value, "integer");
      expect(number.leading + number.digits + number.unit).toBe(expected);
      expect(number.unit).toBe("");
    }
  });
  it("retains small cost magnitude and promotes units after rounding", () => {
    expect(text(0.0001)).toBe("0100µ");
    expect(text(0.01)).toBe("10.0m");
    expect(text(0.000_000_000_001)).toBe("<001n");
  });
  it("keeps the layout bounded for unavailable or extreme values", () => {
    expect(text(NaN)).toBe("----·");
    expect(text(-1)).toBe("----·");
    for (const value of [
      Number.MIN_VALUE,
      0,
      9.999,
      99.999,
      999.999,
      1e20,
      Number.MAX_VALUE,
    ])
      expect(text(value)).toHaveLength(5);
  });
});
