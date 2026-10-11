import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { ProjectPathEntrySchema } from "#gen/cxz/project_svc_pb";
import {
  containerPathQuery,
  downloadArgument,
  matchPaths,
} from "./composer-paths";

describe("container path completion", () => {
  it("opens only for backtick absolute/home paths at a collapsed caret, outside fenced code", () => {
    const text = "Inspect `/workspace/sr` then continue";
    const caret = text.indexOf("sr") + 2;
    expect(containerPathQuery(text, caret)).toMatchObject({
      start: 9,
      text: "/workspace/sr",
      parent: "/workspace/",
      query: "sr",
    });
    expect(containerPathQuery("next\nSee `~", 11)).toMatchObject({
      text: "~",
      parent: "~/",
      query: "",
    });
    for (const value of [
      "/workspace/sr",
      "https://example.com",
      "See `relative",
      "See `~other",
      "See \\`/work",
      "```\n/work",
      "See ``/work",
    ])
      expect(containerPathQuery(value, value.length)).toBeUndefined();
    expect(containerPathQuery(text, caret, caret + 1)).toBeUndefined();
    expect(containerPathQuery(text, text.length)).toBeUndefined();
    expect(containerPathQuery("See `/workspace/src/app.ts`", 14)).toMatchObject(
      { end: 26 },
    );
  });
  it("completes download arguments including quotes/spaces and relative paths within the workspace", () => {
    expect(
      containerPathQuery("/download ", 10, 10, "/workspace"),
    ).toMatchObject({ parent: "/workspace/", query: "", start: 10 });
    expect(containerPathQuery('/download "~/my fi"', 18)).toMatchObject({
      parent: "~/",
      query: "my fi",
      start: 11,
    });
    expect(
      containerPathQuery("/download src/ap", 16, 16, "/workspace"),
    ).toMatchObject({ parent: "/workspace/src/", query: "ap" });
    expect(
      containerPathQuery("/download --cancel", 18, 18, "/workspace"),
    ).toBeUndefined();
  });
  it("fuzzy matches immediate names, prioritizes directories and excludes unsafe entries", () => {
    const entries = [
      "file.txt",
      "src",
      "source.ts",
      "..",
      "bad/name",
      "bad`name",
    ].map((name) =>
      create(ProjectPathEntrySchema, { name, directory: name === "src" }),
    );
    const token = containerPathQuery("See `/sr", 8)!;
    expect(
      matchPaths(entries, token).map((match) => match.command.name),
    ).toEqual(["/src/", "/source.ts"]);
    expect(matchPaths(entries, { ...token, query: "" })).toHaveLength(3);
  });
});

describe("download command arguments", () => {
  it("preserves literal filenames and matches only the download command", () => {
    for (const value of [
      "/download /workspace/a b.txt",
      "/download '/workspace/a b.txt'",
      '/download "/workspace/a b.txt"',
      "/download `/workspace/a b.txt`",
    ])
      expect(downloadArgument(value)).toBe("/workspace/a b.txt");
    expect(downloadArgument("/download ~/notes.txt")).toBe("~/notes.txt");
    expect(downloadArgument("/download README.md")).toBe("README.md");
    expect(downloadArgument("/downloads foo")).toBeUndefined();
    expect(downloadArgument("Please /download foo")).toBeUndefined();
    for (const value of [
      "/download",
      "/download \n/foo",
      "/download /foo\0bar",
    ])
      expect(() => downloadArgument(value)).toThrow();
  });
});
