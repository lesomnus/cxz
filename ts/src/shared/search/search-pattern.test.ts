import { expect, it } from "vitest";
import {
  compileSearchPattern,
  defaultSearchOptions,
  searchMatches,
  type SearchOptions,
} from "./search-pattern";

function matches(
  text: string,
  query: string,
  options: Partial<SearchOptions> = {},
) {
  const compiled = compileSearchPattern(query, {
    ...defaultSearchOptions,
    ...options,
  });
  expect(compiled.error).toBeUndefined();
  return [...searchMatches(text, compiled.pattern)].map((match) => match[0]);
}

it("can distinguish case without changing the literal-search default", () => {
  expect(matches("Network network NETWORK", "network")).toEqual([
    "Network",
    "network",
    "NETWORK",
  ]);
  expect(
    matches("Network network NETWORK", "network", { matchCase: true }),
  ).toEqual(["network"]);
});

it("matches whole words instead of prefixes, suffixes and identifier fragments", () => {
  expect(
    matches("net network ethernet net_net (net) net2", "net", {
      wholeWord: true,
    }),
  ).toEqual(["net", "net"]);
});

it("treats Unicode letters, numbers and combining marks as word characters", () => {
  expect(
    matches("한글 한글자 새한글 (한글) 한글_ 한글2 한글é 한글\u0301", "한글", {
      wholeWord: true,
    }),
  ).toEqual(["한글", "한글"]);
});

it("only interprets regex syntax when enabled", () => {
  expect(matches("a+b ab aaab", "a+b")).toEqual(["a+b"]);
  expect(matches("a+b ab aaab", "a+b", { useRegex: true })).toEqual([
    "ab",
    "aaab",
  ]);
});

it("combines all three options without adding capturing groups", () => {
  expect(
    matches("foo FOO food bar_bar BAR bar", "(foo|bar)", {
      matchCase: true,
      wholeWord: true,
      useRegex: true,
    }),
  ).toEqual(["foo", "bar"]);
  expect(
    matches("foo foo foo bar", "(foo) \\1", {
      wholeWord: true,
      useRegex: true,
    }),
  ).toEqual(["foo foo"]);
});

it("returns invalid patterns as errors instead of throwing while typing", () => {
  for (const query of ["[", "(", "\\"]) {
    for (const wholeWord of [false, true]) {
      const compiled = compileSearchPattern(query, {
        ...defaultSearchOptions,
        useRegex: true,
        wholeWord,
      });
      expect(compiled.error).toBeTruthy();
      expect(compiled.pattern).toBeUndefined();
      expect([...searchMatches("text", compiled.pattern)]).toEqual([]);
    }
  }
});

it("ignores zero-width matches but retains visible matches", () => {
  expect(matches("foo bar", "^|$", { useRegex: true })).toEqual([]);
  expect(matches("foo bar", "(?=foo)|bar", { useRegex: true })).toEqual([
    "bar",
  ]);
  expect(matches("baa", "a*", { useRegex: true })).toEqual(["aa"]);
});

it("can reuse one compiled expression across messages without consuming it", () => {
  const { pattern } = compileSearchPattern("network");
  expect([...searchMatches("network", pattern)]).toHaveLength(1);
  expect([...searchMatches("network", pattern)]).toHaveLength(1);
  expect(pattern?.lastIndex).toBe(0);
});

it("preserves meaningful regex whitespace and returns no empty search", () => {
  expect(matches(" x x ", " x ", { useRegex: true })).toEqual([" x "]);
  expect(compileSearchPattern("")).toEqual({});
});

it("allows regex anchors to address individual lines", () => {
  expect(
    matches("network\nnetworking\nnetwork", "^network$", { useRegex: true }),
  ).toEqual(["network", "network"]);
});
