import { expect, it } from "vitest";
import {
  inlineBacktickEdit,
  inlineCodeRanges,
  listNewlineEdit,
} from "./composer-markdown";

function enter(value: string, start = value.length, end = start) {
  const edit = listNewlineEdit(value, start, end);
  return (
    edit && {
      value: value.slice(0, edit.from) + edit.text + value.slice(edit.to),
      caret: edit.caret,
    }
  );
}

it("pairs inline ticks, skips the closer, and leaves fences/escapes/existing code alone", () => {
  expect(inlineBacktickEdit("Use ", 4)).toEqual({
    text: "``",
    caret: 5,
    skip: false,
  });
  expect(inlineBacktickEdit("Use `code`", 9)).toEqual({
    text: "",
    caret: 10,
    skip: true,
  });
  for (const [value, pos] of [
    ["", 0],
    ["  ", 2],
    ["`", 1],
    ["  ``", 4],
    ["Use \\", 5],
    ["Use `code", 9],
    ["Use `code`", 7],
    ["```\ncode\n```", 8],
  ] as const)
    expect(inlineBacktickEdit(value, pos)).toBeUndefined();
});

it("continues bullet and task lists with their exact space/tab indentation", () => {
  expect(enter("- first")).toEqual({ value: "- first\n- ", caret: 10 });
  expect(enter("  \t- nested")).toEqual({
    value: "  \t- nested\n  \t- ",
    caret: 17,
  });
  expect(enter("\t* item")?.value).toBe("\t* item\n\t* ");
  expect(enter("+ [x] done")?.value).toBe("+ [x] done\n+ [ ] ");
});

it("splits an item at the caret and replaces a selection as one edit", () => {
  expect(enter("- first second", 7)).toEqual({
    value: "- first\n-  second",
    caret: 10,
  });
  expect(enter("- first second", 7, 8)).toEqual({
    value: "- first\n- second",
    caret: 10,
  });
  expect(enter("- item", 1)).toBeUndefined();
});

it("exits empty items and leaves prose and fenced code untouched", () => {
  expect(enter("  - ")).toEqual({ value: "  ", caret: 2 });
  expect(enter("- [ ] ")?.value).toBe("");
  for (const value of [
    "prose",
    "-not a list",
    "```\n- code",
    "````\n- code\n````",
  ])
    expect(enter(value)).toBeUndefined();
  const value = "```\n- code\n```";
  expect(enter(value, value.indexOf("\n```"))).toBeUndefined();
  expect(enter(value + "\n- prose")?.value).toBe(value + "\n- prose\n- ");
});

it("pairs inline backticks including longer delimiters and preserves original bytes", () => {
  const line = "Use `one` and ``two ` ticks``.";
  expect(
    inlineCodeRanges(line).map(({ start, end }) => line.slice(start, end)),
  ).toEqual(["`one`", "``two ` ticks``"]);
  expect(inlineCodeRanges("unclosed `code")).toEqual([]);
  expect(inlineCodeRanges("\\`escaped` plain")).toEqual([]);
  expect(inlineCodeRanges("``unclosed then `valid` code")).toEqual([
    { start: 16, end: 23 },
  ]);
  const escapedClosing = "`code\\` tail";
  expect(inlineCodeRanges(escapedClosing)).toEqual([{ start: 0, end: 7 }]);
});
