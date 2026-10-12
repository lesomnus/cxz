import { expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import { mentionMatches, mentionQuery, redactQuery } from "./composer-mentions";
it("completes only plain-text mentions and preserves their suffix", () => {
  expect(mentionQuery("ask @oa now", 7, 7)).toEqual({
    start: 4,
    end: 7,
    query: "@oa",
  });
  expect(mentionQuery("@oak", 2, 2)).toEqual({ start: 0, end: 4, query: "@o" });
  for (const value of ["mail@oak", "`@oak", "/@oak", "```\n@oak"])
    expect(mentionQuery(value, value.length, value.length)).toBeUndefined();
  expect(mentionQuery("@oak", 1, 4)).toBeUndefined();
  const inside = "```ts\nconst x = `value\n@oak\n```";
  expect(
    mentionQuery(
      inside,
      inside.indexOf("@oak") + 4,
      inside.indexOf("@oak") + 4,
    ),
  ).toBeUndefined();
  const after = inside + "\nask @oak";
  expect(mentionQuery(after, after.length, after.length)?.query).toBe("@oak");
});
it("offers secret input only for a plain inline directive", () => {
  expect(redactQuery("pass /@red", 10, 10)).toEqual({
    start: 5,
    end: 10,
    query: "/@red",
  });
  expect(redactQuery("`/@red", 6, 6)).toBeUndefined();
  expect(redactQuery("/@red", 5, 5)).toBeUndefined();
});
it("keeps mentions within the current project and excludes self/archive", () => {
  const project = { id: new Uint8Array([1]), runtimeId: "p" };
  const current = create(SessionSchema, {
    runtimeId: "self",
    listed: true,
    alias: "self",
    project,
  });
  const rows = [
    create(SessionSchema, {
      runtimeId: "oak",
      alias: "oak",
      name: "Research",
      listed: true,
      project,
    }),
    create(SessionSchema, {
      runtimeId: "other",
      alias: "oak-other",
      listed: true,
      project: { id: new Uint8Array([2]) },
    }),
    create(SessionSchema, {
      runtimeId: "gone",
      alias: "oak-gone",
      listed: false,
      project,
    }),
    current,
  ];
  expect(
    mentionMatches(rows, current, "@oa").map((m) => m.command.name),
  ).toEqual(["@oak"]);
});
