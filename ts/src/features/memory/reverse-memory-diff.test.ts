import { expect, test } from "vitest";
import { reverseMemoryDiff } from "./reverse-memory-diff";
import {
  memoryParagraphBefore,
  memoryParagraphAfter,
} from "#src/storybook/memory-long-paragraph.ts";

const patch = (body: string) => `--- old\n+++ new\n${body}`;
test("reconstructs before text from replacement and context lines", () => {
  expect(
    reverseMemoryDiff(
      "a\nnew\nz\n",
      patch("@@ -1,3 +1,3 @@\n a\n-old\n+new\n z\n"),
    ),
  ).toBe("a\nold\nz\n");
});
test("preserves both missing final newlines and EOF-only changes", () => {
  expect(
    reverseMemoryDiff(
      "new",
      patch(
        "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n",
      ),
    ),
  ).toBe("old");
  expect(
    reverseMemoryDiff(
      "same\n",
      patch("@@ -1 +1 @@\n-same\n\\ No newline at end of file\n+same\n"),
    ),
  ).toBe("same");
  expect(
    reverseMemoryDiff(
      "same",
      patch("@@ -1 +1 @@\n-same\n+same\n\\ No newline at end of file\n"),
    ),
  ).toBe("same\n");
});
test("keeps CRLF bytes instead of normalizing the original", () => {
  expect(
    reverseMemoryDiff("new\r\n", patch("@@ -1 +1 @@\n-old\r\n+new\r\n")),
  ).toBe("old\r\n");
});
test("supports empty files and deletion-only hunks", () => {
  expect(reverseMemoryDiff("new\n", patch("@@ -0,0 +1 @@\n+new\n"))).toBe("");
  expect(reverseMemoryDiff("", patch("@@ -1 +0,0 @@\n-old\n"))).toBe("old\n");
});
test("restores multiple hunks while retaining unchanged gaps", () => {
  expect(
    reverseMemoryDiff(
      "one\nnew\nthree\nfour\nsix\n",
      patch("@@ -2 +2 @@\n-old\n+new\n@@ -5 +5 @@\n-five\n+six\n"),
    ),
  ).toBe("one\nold\nthree\nfour\nfive\n");
});
test("rejects stale contents, invalid counts and reordered hunks", () => {
  expect(() =>
    reverseMemoryDiff("stale\n", patch("@@ -1 +1 @@\n-old\n+new\n")),
  ).toThrow();
  expect(() =>
    reverseMemoryDiff("new\n", patch("@@ -1,2 +1 @@\n-old\n+new\n")),
  ).toThrow();
  expect(() =>
    reverseMemoryDiff(
      "a\nb\n",
      patch("@@ -2 +2 @@\n-b\n+b\n@@ -1 +1 @@\n-a\n+a\n"),
    ),
  ).toThrow();
});
test("reconstructs the reported long paragraph exactly", () => {
  const text = patch(
    `@@ -1 +1 @@\n-${memoryParagraphBefore}\n\\ No newline at end of file\n+${memoryParagraphAfter}\n\\ No newline at end of file\n`,
  );
  expect(reverseMemoryDiff(memoryParagraphAfter, text)).toBe(
    memoryParagraphBefore,
  );
});
