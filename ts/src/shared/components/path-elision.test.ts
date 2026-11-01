import { describe, expect, it } from "vitest";
import { elidePath } from "./path-elision";

const measure = (value: string) => value.length;
const original =
  "/home/somnus/workspaces/github.com/lesomnus/cxz/.devcontainer/devcontainer.json";
const rendered = (path: ReturnType<typeof elidePath>) =>
  path.prefix +
  (path.middle ? `⋯${path.separator}` : "") +
  path.leaf +
  (path.end ? "⋯" : "");

describe("middle path elision", () => {
  it("keeps fitting paths verbatim, including trailing separators", () => {
    for (const path of [
      original,
      "/home/somnus/",
      "./config.json",
      "C:\\Users\\somnus\\",
    ]) {
      expect(rendered(elidePath(path, path.length, measure, 1))).toBe(path);
    }
  });

  it("uses the longest leading prefix that fits alongside the entire leaf", () => {
    const expected = "/home/somnus/workspaces/⋯/devcontainer.json";
    const result = elidePath(original, expected.length, measure, 1);
    expect(rendered(result)).toBe(expected);
    expect(result.end).toBe(false);
    expect(rendered(elidePath(original, expected.length - 1, measure, 1))).toBe(
      "/home/somnus/⋯/devcontainer.json",
    );
  });

  it("preserves the first two directory entries before shortening the leaf", () => {
    const expected = "/home/somnus/⋯/devco⋯";
    expect(rendered(elidePath(original, expected.length, measure, 1))).toBe(
      expected,
    );
  });

  it("does not insert a middle mark when there are no omitted directories", () => {
    expect(
      rendered(elidePath("/home/somnus/devcontainer.json", 20, measure, 1)),
    ).toBe("/home/somnus/devcon⋯");
    expect(rendered(elidePath("devcontainer.json", 6, measure, 1))).toBe(
      "devco⋯",
    );
  });

  it("retains Windows roots and separators", () => {
    const path = "C:\\Users\\somnus\\workspaces\\cxz\\devcontainer.json";
    const expected = "C:\\Users\\somnus\\⋯\\devco⋯";
    expect(rendered(elidePath(path, expected.length, measure, 1))).toBe(
      expected,
    );
  });

  it("measures actual glyph widths and reserves room for both symbols", () => {
    const proportional = (text: string) =>
      Array.from(text).reduce(
        (width, char) => width + (char === "m" ? 3 : 1),
        0,
      );
    const result = elidePath(original, 28, proportional, 2);
    expect(result.prefix).toBe("/home/somnus/");
    expect(
      proportional(result.prefix + result.separator + result.leaf) + 4,
    ).toBeLessThanOrEqual(28);
  });

  it("shortens leaves only at grapheme boundaries", () => {
    const path = "/home/somnus/👩‍💻é-file.json";
    const graphemes = (text: string) =>
      Array.from(
        new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(
          text,
        ),
      ).length;
    expect(rendered(elidePath(path, 16, graphemes, 1))).toBe(
      "/home/somnus/👩‍💻é⋯",
    );
  });

  it("keeps a trailing directory separator with a fitting leaf", () => {
    const path = original + "/";
    const expected = "/home/somnus/⋯/devcontainer.json/";
    expect(rendered(elidePath(path, expected.length, measure, 1))).toBe(
      expected,
    );
  });
});
