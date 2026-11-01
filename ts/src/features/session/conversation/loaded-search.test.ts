import { expect, it } from "vitest";
import { searchText } from "./loaded-search";

const marked = (result: ReturnType<typeof searchText>) =>
  result?.snippetMatches.map(({ start, end }) =>
    result.snippet.slice(start, end),
  );

it("keeps original casing and every matched span in a snippet", () => {
  const result = searchText("Network: network, NETWORK.", "nEtWoRk");
  expect(marked(result)).toEqual(["Network", "network", "NETWORK"]);
  expect(result?.occurrences).toBe(3);
});

it("treats regex punctuation as literal text", () => {
  const result = searchText("a+b[0] differs from ab0 and a+b[0].", "a+b[0]");
  expect(marked(result)).toEqual(["a+b[0]", "a+b[0]"]);
  expect(searchText("ab0", "a+b[0]")).toBeUndefined();
});

it("keeps Unicode match positions after emoji and within Korean text", () => {
  const result = searchText("🧩 한글 검색과 한글 본문", "한글");
  expect(result?.snippetMatches).toEqual([
    { start: 3, end: 5 },
    { start: 10, end: 12 },
  ]);
  expect(marked(result)).toEqual(["한글", "한글"]);
});

it("counts matches beyond the excerpt but only marks visible source matches", () => {
  const text = "prefix ".repeat(30) + "match " + "suffix ".repeat(50) + "match";
  const result = searchText(text, "match");
  expect(result?.occurrences).toBe(2);
  expect(result?.snippet).toMatch(/^….*match.*…$/);
  expect(result?.snippetMatches).toEqual([{ start: 81, end: 86 }]);
  expect(marked(result)).toEqual(["match"]);
});

it("does not highlight the synthetic ellipsis before an excerpt", () => {
  const result = searchText("a".repeat(100) + "… rest", "…");
  expect(result?.snippetMatches).toEqual([{ start: 81, end: 82 }]);
  expect(marked(result)).toEqual(["…"]);
});

it("clips a long match to the excerpt without including its trailing ellipsis", () => {
  const result = searchText("a".repeat(120) + "x".repeat(200), "x".repeat(200));
  expect(result?.snippetMatches).toEqual([{ start: 81, end: 241 }]);
  expect(marked(result)).toEqual(["x".repeat(160)]);
  expect(result?.snippet.endsWith("…")).toBe(true);
});

it("returns no result for empty or unmatched queries", () => {
  expect(searchText("text", "")).toBeUndefined();
  expect(searchText("text", "missing")).toBeUndefined();
});
