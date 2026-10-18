import { expect, test } from "vitest";
import {
  client,
  forget,
  list,
  remember,
  storageKey,
  transportOf,
} from "./connections";

function storage(): Storage {
  const map = new Map<string, string>();
  return {
    get length() {
      return map.size;
    },
    clear: () => map.clear(),
    getItem: (k: string) => map.get(k) ?? null,
    key: (i: number) => [...map.keys()][i] ?? null,
    removeItem: (k: string) => void map.delete(k),
    setItem: (k: string, v: string) => void map.set(k, v),
  };
}

const origin = "https://cxz.example:7350";

test("an empty browser is attached to exactly one connection, and cannot forget it", () => {
  const s = storage();
  const all = list(origin, s);
  expect(all).toHaveLength(1);
  expect(all[0].target).toBe(origin);
  expect(all[0].readOnly).toBe(true);
  expect(all[0].isDefault).toBe(true);
  expect(all[0].transport).toBe("origin");
  expect(all[0].name).toBe("cxz.example:7350");

  forget(origin, origin, s);
  expect(list(origin, s)).toHaveLength(1);
  // Nothing was written to forget it, either.
  expect(s.getItem(storageKey)).toBe(null);
});

test("a remembered connection keeps a name and a target and nothing else", () => {
  const s = storage();
  remember({ name: "work", target: "https://build.example:7350" }, origin, s);
  const stored: unknown = JSON.parse(s.getItem(storageKey) ?? "[]");
  expect(stored).toEqual([
    { name: "work", target: "https://build.example:7350" },
  ]);

  const all = list(origin, s);
  expect(all.map((c) => c.target)).toEqual([
    origin,
    "https://build.example:7350",
  ]);
  expect(all[1].readOnly).toBe(false);
  expect(all[1].isDefault).toBe(false);

  forget("https://build.example:7350", origin, s);
  expect(list(origin, s)).toHaveLength(1);
});

test("a credential that reached storage by some other means is not carried", () => {
  const s = storage();
  s.setItem(
    storageKey,
    JSON.stringify([
      { name: "work", target: "https://build.example:7350", token: "secret" },
    ]),
  );
  const all = list(origin, s);
  expect(all).toHaveLength(2);
  expect(JSON.stringify(all)).not.toContain("secret");
  remember({ name: "lan", target: "https://lan.example:7350" }, origin, s);
  expect(s.getItem(storageKey) ?? "").not.toContain("secret");
});

test("the origin cannot be remembered twice, under any name", () => {
  const s = storage();
  remember({ name: "myself", target: origin }, origin, s);
  expect(s.getItem(storageKey)).toBe(null);
  expect(list(origin, s)).toHaveLength(1);
  // Nor by having been stored before this rule existed.
  s.setItem(storageKey, JSON.stringify([{ name: "myself", target: origin }]));
  expect(list(origin, s)).toHaveLength(1);
});

test("unreadable storage is an empty list, not a broken page", () => {
  const s = storage();
  s.setItem(storageKey, "{not json");
  expect(list(origin, s)).toHaveLength(1);
  s.setItem(storageKey, JSON.stringify({ name: "work" }));
  expect(list(origin, s)).toHaveLength(1);
});

test("a target is shown as the transport it is written as", () => {
  expect(transportOf("https://cxz.example:7350")).toBe("origin");
  expect(transportOf("http://127.0.0.1:7350")).toBe("origin");
  expect(transportOf("ssh://me@build")).toBe("ssh");
  expect(transportOf("mtls://10.0.0.5:7349")).toBe("mtls");
  expect(transportOf("tcp://10.0.0.5:7349")).toBe("tcp");
  expect(transportOf("local://")).toBe("local");
  expect(transportOf("nonsense")).toBe("");
});

test("the UI reads it through a generated client, like any other service", async () => {
  const s = storage();
  remember({ name: "work", target: "ssh://me@build" }, origin, s);
  const reply = await client(origin, s).list({});
  expect(reply.items.map((c) => c.name)).toEqual(["cxz.example:7350", "work"]);
  expect(reply.items[1].transport).toBe("ssh");
});
