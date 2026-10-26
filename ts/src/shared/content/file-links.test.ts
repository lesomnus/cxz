import { expect, it } from "vitest";
import { workspaceFilePath } from "./file-links";

it.each([
  ["/tmp/report.md", "/tmp/report.md"],
  ["/workspace/file.ts:42", "/workspace/file.ts"],
  ["/workspace/file.ts:42:3#L42", "/workspace/file.ts"],
  ["/tmp/file%3A42", "/tmp/file:42"],
  ["file:///workspace/file.ts", "/workspace/file.ts"],
  ["file:///workspace/file.ts:42:3", "/workspace/file.ts"],
  ["file://localhost/workspace/a%20file.ts", "/workspace/a file.ts"],
  ["file://other-host/workspace/file.ts", undefined],
  ["file:///tmp/%00report", undefined],
  [
    "/workspace/%ED%95%9C%EA%B8%80%20report.md#L10",
    "/workspace/한글 report.md",
  ],
  ["/tmp/name%23one%3F.md", "/tmp/name#one?.md"],
  ["/tmp/100%.txt", "/tmp/100%.txt"],
  ["https://example.com/tmp/report.md", undefined],
  ["//example.com/tmp/report.md", undefined],
  ["/\\example.com/tmp/report.md", undefined],
  ["relative/report.md", undefined],
  ["#section", undefined],
  ["javascript:alert(1)", undefined],
  ["/tmp/%00report", undefined],
])("resolves %s as %s", (href, path) => {
  expect(workspaceFilePath(href)).toBe(path);
});
