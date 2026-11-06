import { afterEach, expect, it, vi } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";
import { ClientErrors, isAPIError } from "./client-errors";

afterEach(() => vi.useRealTimers());
function storage() {
  const values = new Map<string, string>();
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    values,
  };
}
it("deduplicates an active failure and retains a separate episode after recovery", () => {
  const errors = new ClientErrors();
  const error = new ConnectError("offline", Code.Unavailable);
  errors.report("account:A", error, { source: "rpc" });
  errors.report("account:A", error, { source: "rpc" });
  expect(errors.snapshot()).toHaveLength(1);
  expect(errors.snapshot()[0]).toMatchObject({
    status: "active",
    count: 2,
    code: "Unavailable",
  });
  errors.resolve("account:B");
  expect(errors.snapshot()[0].status).toBe("active");
  errors.resolve("account:A");
  expect(errors.snapshot()[0]).toMatchObject({ status: "resolved", count: 2 });
  errors.report("account:A", error, { source: "rpc" });
  expect(errors.snapshot().map((e) => e.status)).toEqual([
    "active",
    "resolved",
  ]);
});
it("restores redacted active/resolved history, keeps clear-history bounded and ignores cancellation", () => {
  vi.useFakeTimers();
  const disk = storage();
  const errors = new ClientErrors({ storage: disk, limit: 3 });
  const input =
    "Bearer private-credential token=private-password https://user:pass@example.com/api?secret=private-query eyJhbG.fake.signature";
  errors.report("A", new Error(input), {
    source: "rpc",
    connection: "https://user:pass@example.com/api?secret=private-query",
  });
  errors.report("cancel", new ConnectError("canceled", Code.Canceled));
  errors.report("B", new Error("second"), { source: "rpc" });
  errors.resolve("A");
  errors.flush();
  const raw = [...disk.values.values()].join("");
  for (const secret of [
    "private-credential",
    "private-password",
    "private-query",
    "user:pass",
    "eyJhbG.fake.signature",
  ])
    expect(raw).not.toContain(secret);
  const restored = new ClientErrors({ storage: disk, limit: 3 });
  expect(restored.snapshot().map((e) => e.status)).toEqual([
    "active",
    "resolved",
  ]);
  restored.clearHistory();
  restored.resolve("B");
  restored.flush();
  expect(new ClientErrors({ storage: disk }).snapshot()).toHaveLength(1);
  expect(restored.snapshot()[0]).toMatchObject({
    status: "resolved",
    message: "second",
  });
  for (let i = 0; i < 10; i++) restored.report(String(i), new Error(String(i)));
  expect(restored.snapshot()).toHaveLength(3);
  restored.flush();
});
it("keeps dismissed entries distinct from confirmed recovery and tolerates storage/listener failures", () => {
  const errors = new ClientErrors({
    storage: {
      getItem: () => {
        throw Error("denied");
      },
      setItem: () => {
        throw Error("full");
      },
    },
  });
  errors.subscribe(() => {
    throw Error("view");
  });
  const id = errors.report("A", new Error("failure"))!;
  errors.dismiss(id);
  expect(errors.snapshot()[0].status).toBe("dismissed");
  expect(() => errors.flush()).not.toThrow();
  errors.report("A", new Error("failure"));
  expect(errors.snapshot()).toHaveLength(2);
  errors.flush();
});
it("recognizes RPC feedback without hiding ordinary validation messages", () => {
  expect(
    isAPIError(
      "Cannot load: ConnectError: [unavailable] offline",
      new ClientErrors(),
    ),
  ).toBe(true);
  expect(isAPIError("Alias must contain letters", new ClientErrors())).toBe(
    false,
  );
  expect(
    isAPIError("Alias must not contain [not_found]", new ClientErrors()),
  ).toBe(false);
});
