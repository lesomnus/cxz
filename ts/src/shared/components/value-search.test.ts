import { describe, expect, it } from "vitest";
import {
  matchValueQuery,
  matchesValueQuery,
} from "../../../packages/nox/src/components/value-menu/value-search";
describe("project query matching", () => {
  it("matches substrings, ordered abbreviations, unicode and case-insensitive queries", () => {
    for (const query of ["", "Demo", "PROJECT", "dmprj", "d p j", "ＤＥＭＯ"])
      expect(matchesValueQuery("Demo project", query)).toBe(true);
    expect(matchesValueQuery("프로젝트", "프젝")).toBe(true);
    expect(matchesValueQuery("Demo project", "jpd")).toBe(false);
    expect(matchesValueQuery("Demo project", "missing")).toBe(false);
  });
});

it("returns the matching displayed characters, including normalized graphemes", () => {
  const hit = (text: string, query: string) =>
    matchValueQuery(text, query)?.map(({ start, end }) =>
      text.slice(start, end),
    );
  expect(hit("Docs playground", "dcsplg")).toEqual(["D", "cs", "pl", "g"]);
  expect(hit("abacus abc", "abc")).toEqual(["abc"]);
  expect(hit("ＡＢＣ", "abc")).toEqual(["ＡＢＣ"]);
  expect(hit("Cafe\u0301", "é")).toEqual(["e\u0301"]);
  expect(hit("ﬃ", "ffi")).toEqual(["ﬃ"]);
  expect(hit("프로젝트", "프젝")).toEqual(["프", "젝"]);
  expect(hit("hello", "zzz")).toBeUndefined();
});
