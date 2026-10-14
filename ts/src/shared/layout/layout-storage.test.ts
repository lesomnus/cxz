import { afterEach, expect, it, vi } from "vitest";
import {
  LAYOUT_STORAGE_KEY,
  loadPaneWidth,
  savePaneWidth,
} from "./layout-storage";

afterEach(() => vi.unstubAllGlobals());
function storage(raw = "{}") {
  let value = raw;
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => (key === LAYOUT_STORAGE_KEY ? value : null),
    setItem: (key: string, next: string) => {
      expect(key).toBe(LAYOUT_STORAGE_KEY);
      value = next;
    },
  });
  return () => JSON.parse(value);
}
it("keeps both pane widths independently and resets only the chosen pane", () => {
  const saved = storage();
  savePaneWidth("session", 940);
  savePaneWidth("file", 240);
  expect(loadPaneWidth("session")).toBe(940);
  expect(loadPaneWidth("file")).toBe(240);
  savePaneWidth("session", undefined);
  expect(loadPaneWidth("session")).toBeUndefined();
  expect(saved()).toEqual({ file: 240 });
});
it("ignores invalid saved values and recovers a malformed document on resize", () => {
  storage('{"session":-2,"file":"240"}');
  expect(loadPaneWidth("session")).toBeUndefined();
  expect(loadPaneWidth("file")).toBeUndefined();
  const saved = storage("invalid");
  expect(loadPaneWidth("session")).toBeUndefined();
  savePaneWidth("session", 900);
  expect(saved()).toEqual({ session: 900 });
  savePaneWidth("session", Infinity);
  expect(saved()).toEqual({ session: 900 });
});
it("keeps storage failures from breaking resizing", () => {
  vi.stubGlobal("localStorage", {
    getItem() {
      throw Error("blocked");
    },
    setItem() {
      throw Error("full");
    },
  });
  expect(loadPaneWidth("session")).toBeUndefined();
  expect(() => savePaneWidth("session", 900)).not.toThrow();
});
