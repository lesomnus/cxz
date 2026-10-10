import { expect, it } from "vitest";
import { indentEdit } from "./composer-indent";

function apply(value: string, start: number, end: number, outdent = false) {
  const edit = indentEdit(value, start, end, outdent);
  return {
    value: value.slice(0, edit.from) + edit.text + value.slice(edit.to),
    start: edit.start,
    end: edit.end,
  };
}

it("inserts two spaces at the cursor without replacing neighboring text", () => {
  expect(apply("alpha", 2, 2)).toEqual({ value: "al  pha", start: 4, end: 4 });
  expect(apply("", 0, 0)).toEqual({ value: "  ", start: 2, end: 2 });
});
it("indents selected logical lines, excluding a line starting at the selection end", () => {
  expect(apply("one\ntwo\nthree", 0, 8)).toEqual({
    value: "  one\n  two\nthree",
    start: 2,
    end: 12,
  });
  expect(apply("one", 1, 2)).toEqual({ value: "  one", start: 3, end: 4 });
});
it("outdents spaces or an existing tab and preserves selected original text", () => {
  expect(apply("  one\n\ttwo\nthree", 2, 10, true)).toEqual({
    value: "one\ntwo\nthree",
    start: 0,
    end: 7,
  });
  expect(apply(" x", 0, 0, true)).toEqual({ value: "x", start: 0, end: 0 });
  expect(apply("  x", 1, 1, true)).toEqual({ value: "x", start: 0, end: 0 });
});
it("handles blank first lines, trailing newlines and a no-op outdent", () => {
  expect(apply("\nnext", 0, 1)).toEqual({
    value: "  \nnext",
    start: 2,
    end: 3,
  });
  expect(apply("\nnext", 0, 0, true)).toEqual({
    value: "\nnext",
    start: 0,
    end: 0,
  });
  expect(apply("a\n", 0, 2)).toEqual({ value: "  a\n", start: 2, end: 4 });
  expect(apply("a\n", 2, 2)).toEqual({ value: "a\n  ", start: 4, end: 4 });
});
it("keeps Unicode and paste placeholders intact while adjusting selection offsets", () => {
  const token = "[Paste 1234abcd · 4L · 20B]";
  const value = `한글 😀\n${token}`;
  const edit = apply(value, 0, value.length);
  expect(edit.value).toBe(`  한글 😀\n  ${token}`);
  expect(edit.end).toBe(value.length + 4);
  expect(apply(edit.value, edit.start, edit.end, true).value).toBe(value);
});
it("uses configurable space indentation and removes the configured width", () => {
  const edit = indentEdit("one\ntwo", 0, 7, false, {
    indentSize: 4,
    insertSpaces: true,
  });
  expect(edit.text).toBe("    one\n    two");
  expect(edit.end).toBe(15);
  expect(
    indentEdit(edit.text, edit.start, edit.end, true, {
      indentSize: 4,
      insertSpaces: true,
    }).text,
  ).toBe("one\ntwo");
});
it("inserts real tabs and can outdent mixed leading tabs and spaces", () => {
  const options = { indentSize: 6, insertSpaces: false };
  expect(indentEdit("x", 0, 0, false, options).text).toBe("\t");
  expect(indentEdit("one\ntwo", 0, 7, false, options).text).toBe(
    "\tone\n\ttwo",
  );
  expect(indentEdit("\tone\n      two", 0, 14, true, options).text).toBe(
    "one\ntwo",
  );
});
