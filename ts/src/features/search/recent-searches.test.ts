import { afterEach, expect, it, vi } from "vitest";
import {
  createSearchHistory,
  SearchHistory,
  searchHistoryLifetime,
  type RecentSearch,
} from "./recent-searches";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const origin = "https://example.test";
const now = 1795000000000;
const query: RecentSearch = {
  id: "new",
  query: "timeout",
  project: "work",
  projectName: "Work",
  match: "fuzzy",
  ignoreCase: true,
  since: "2026-11-12",
  until: "2026-11-19",
  time: now,
  hits: 100,
  hasMore: true,
  partial: false,
};

function fakeStorage() {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    key: (i: number) => [...values.keys()][i] ?? null,
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  };
}

it("restores only the authenticated connection's searches and prunes expired storage", () => {
  const storage = fakeStorage();
  vi.stubGlobal("localStorage", storage);
  vi.useFakeTimers();
  vi.setSystemTime(now);
  createSearchHistory(origin, "A").remember(query, false);
  expect(createSearchHistory(origin, "A").list()).toEqual([query]);
  expect(createSearchHistory(origin, "B").list()).toEqual([]);
  expect(createSearchHistory(origin + "/other", "A").list()).toEqual([]);
  expect(createSearchHistory(origin).list()).toEqual([]);
  vi.setSystemTime(now + searchHistoryLifetime);
  expect(createSearchHistory(origin, "B").list()).toEqual([]);
  expect(storage.length).toBe(0);
});

it("updates pagination in place and leaves a newer repeat alone", () => {
  const history = new SearchHistory();
  history.remember(query, false, now);
  const page = { ...query, hits: 120, hasMore: false };
  expect(history.remember(page, true, now + 1)).toEqual([page]);
  const repeat = {
    ...query,
    id: "repeat",
    time: now + 2,
    hits: 0,
    hasMore: false,
  };
  expect(history.remember(repeat, false, now + 2)).toEqual([repeat]);
  expect(history.remember(page, true, now + 3)).toEqual([repeat]);
  expect(history.clear()).toEqual([]);
  expect(history.remember(repeat, true, now + 3)).toEqual([]);
});
