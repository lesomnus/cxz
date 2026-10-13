import { describe, expect, it, vi } from "vitest";
import { installReactPerformanceBudget } from "./react-performance";

function fixture() {
  const entries: PerformanceMeasure[] = [];
  const measure = vi.fn(
    (name: string, options?: PerformanceMeasureOptions | string) => {
      const entry = {
        name,
        entryType: "measure",
        detail:
          typeof options === "object" ? structuredClone(options.detail) : null,
      } as PerformanceMeasure;
      entries.push(entry);
      return entry;
    },
  );
  const target = {
    measure,
    getEntriesByName: (name: string) =>
      entries.filter((entry) => entry.name === name),
    clearMeasures: (name: string) => {
      for (let index = entries.length - 1; index >= 0; index--)
        if (entries[index].name === name) entries.splice(index, 1);
    },
  } as unknown as Performance;
  return { target, entries, measure };
}
const options = {
  start: 0,
  end: 1,
  detail: {
    devtools: { track: "Components ⚛", properties: [["message", "hello"]] },
  },
};

describe("React development timing budget", () => {
  it("bounds retained React timings and large changed props without changing app timings", () => {
    const { target, entries, measure } = fixture();
    const dispose = installReactPerformanceBudget(target);
    const manual = {
      start: 0,
      end: 1,
      detail: { app: "keep this", text: "x".repeat(10000) },
    };
    target.measure("app", manual);
    const large = {
      ...options,
      detail: {
        devtools: {
          ...options.detail.devtools,
          properties: Array.from({ length: 2000 }, () => [
            "value",
            "x".repeat(10000),
          ]),
        },
      },
    };
    for (let index = 0; index < 1000; index++)
      target.measure(`React ${index}`, large);
    expect(entries.length).toBeLessThanOrEqual(129);
    expect(entries[0].detail).toEqual(manual.detail);
    const recorded = entries.at(-1)!.detail.devtools.properties;
    expect(recorded.length).toBeLessThan(
      large.detail.devtools.properties.length,
    );
    expect(recorded[0][1].length).toBeLessThan(1000);
    expect(large.detail.devtools.properties).toHaveLength(2000);
    target.measure("app marks", "start", "end");
    expect(measure).toHaveBeenLastCalledWith("app marks", "start", "end");
    dispose();
    expect(target.measure).toBe(measure);
    expect(entries.map((entry) => entry.name)).toEqual(["app", "app marks"]);
  });
  it("preserves third-party entries even when their label matches a React label", () => {
    const { target, entries } = fixture();
    const dispose = installReactPerformanceBudget(target);
    target.measure("shared", { detail: { app: "owned elsewhere" } });
    target.measure("shared", options);
    for (let index = 0; index < 200; index++)
      target.measure(`React ${index}`, options);
    expect(entries.find((entry) => entry.detail?.app)?.detail.app).toBe(
      "owned elsewhere",
    );
    dispose();
  });
  it("retries a React-only clone failure without props and leaves other errors visible", () => {
    const { target, measure } = fixture();
    const dispose = installReactPerformanceBudget(target);
    measure.mockImplementationOnce(() => {
      throw new DOMException("out of memory", "DataCloneError");
    });
    expect(() => target.measure("React", options)).not.toThrow();
    expect(measure.mock.calls.at(-1)?.[1]).toMatchObject({
      start: 0,
      end: 1,
      detail: { devtools: { track: "Components ⚛" } },
    });
    expect(
      (measure.mock.calls.at(-1)?.[1] as PerformanceMeasureOptions).detail
        .devtools.properties,
    ).toBeUndefined();
    measure.mockImplementationOnce(() => {
      throw new DOMException("bad data", "DataCloneError");
    });
    expect(() => target.measure("app", { detail: { app: true } })).toThrow(
      "bad data",
    );
    measure.mockImplementationOnce(() => {
      throw new Error("invalid timing");
    });
    expect(() => target.measure("React", options)).toThrow("invalid timing");
    expect(installReactPerformanceBudget(target)).toBe(dispose);
    dispose();
  });
});
