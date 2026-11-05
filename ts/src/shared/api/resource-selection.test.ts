import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ResourceSelection,
  createResourceSelection,
} from "./resource-selection";

afterEach(() => vi.unstubAllGlobals());

const storage = () => {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
};

describe("shared resource selection", () => {
  it("scopes persisted browser selections by server and authenticated reader", () => {
    vi.stubGlobal("sessionStorage", storage());
    createResourceSelection("https://one.invalid", "reader-A").chooseSession(
      "P",
      "S",
      "R",
    );
    expect(
      createResourceSelection("https://one.invalid", "reader-A").snapshot()
        .session,
    ).toBe("S");
    expect(
      createResourceSelection("https://one.invalid", "reader-B").snapshot()
        .project,
    ).toBe("");
    expect(
      createResourceSelection("https://two.invalid", "reader-A").snapshot()
        .project,
    ).toBe("");
    expect(
      createResourceSelection("https://one.invalid").snapshot().project,
    ).toBe("");
  });
  it("retains the session within a project, clears it on project change and notifies only on changes", () => {
    const store = new ResourceSelection();
    const changed = vi.fn();
    const stop = store.subscribe(changed);
    store.chooseSession("P", "session-uuid", "runtime");
    store.chooseProject("P");
    store.chooseSession("P", "session-uuid", "runtime");
    expect(changed).toHaveBeenCalledTimes(1);
    expect(store.snapshot()).toEqual({
      project: "P",
      session: "session-uuid",
      assignee: "runtime",
    });
    store.chooseProject("Q");
    expect(store.snapshot()).toEqual({
      project: "Q",
      session: "",
      assignee: "",
    });
    stop();
    store.clear();
    expect(changed).toHaveBeenCalledTimes(2);
  });
  it("restores tab-local state independently for each connection and clears it on sign-out", () => {
    const disk = storage();
    const a = new ResourceSelection("connection-A", disk);
    a.chooseSession("P", "session-uuid", "runtime");
    const restored = new ResourceSelection("connection-A", disk);
    expect(restored.snapshot()).toEqual(a.snapshot());
    expect(new ResourceSelection("connection-B", disk).snapshot().project).toBe(
      "",
    );
    restored.clear();
    expect(disk.getItem("connection-A")).toBeNull();
    expect(new ResourceSelection("connection-A", disk).snapshot().project).toBe(
      "",
    );
  });
  it("ignores malformed storage and keeps working when reads and writes are blocked", () => {
    const disk = storage();
    for (const raw of [
      "not json",
      "null",
      "[]",
      '{"project":123}',
      JSON.stringify({ project: "P", session: "", assignee: "x".repeat(257) }),
    ]) {
      disk.setItem("key", raw);
      expect(new ResourceSelection("key", disk).snapshot().project).toBe("");
    }
    const blocked = () => {
      throw Error("Storage blocked");
    };
    const store = new ResourceSelection("key", {
      getItem: blocked,
      setItem: blocked,
      removeItem: blocked,
    });
    store.chooseSession("P", "S", "R");
    expect(store.snapshot().session).toBe("S");
    store.clear();
    expect(store.snapshot().project).toBe("");
  });
  it("keeps memory-only connections and previews isolated", () => {
    const a = new ResourceSelection(),
      b = new ResourceSelection();
    a.chooseSession("P", "S", "R");
    expect(b.snapshot().project).toBe("");
    a.chooseSession("P", "", "");
    expect(a.snapshot()).toEqual({ project: "P", session: "", assignee: "" });
  });
});
