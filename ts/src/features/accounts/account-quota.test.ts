import { create } from "@bufbuild/protobuf";
import { expect, it, vi } from "vitest";
import { SessionEventSchema, SessionSchema } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { accountQuota, readAccountQuota } from "./account-quota";

const event = (timeMs: number, seq: bigint, text: string, payload: unknown) =>
  create(SessionEventSchema, {
    timeMs: BigInt(timeMs),
    seq,
    kind: "usage",
    text,
    payload: new TextEncoder().encode(JSON.stringify(payload)),
  });
const codex = (usedPercent: number) => ({
  rateLimits: {
    primary: { usedPercent, windowDurationMins: 300 },
    secondary: { usedPercent: 40, windowDurationMins: 10080 },
  },
});

it("orders account snapshots by time across independent session sequences and selects the constraining window", () => {
  const events = [
    event(2000, 1n, "account/rateLimits/updated", codex(20)),
    event(1000, 900n, "account/rateLimits/updated", codex(95)),
  ];
  const quota = accountQuota("codex", events);
  expect(quota?.remaining).toBe(60);
  expect(quota?.period).toBe("weekly");
  expect(events[0].timeMs).toBe(2000n);
});

it("retains numeric Claude windows across failed probes and partial notifications without inventing reset usage", () => {
  const events = [
    event(1000, 1n, "get_usage", {
      rate_limits: {
        five_hour: { utilization: 80, resets_at: 10 },
        seven_day: { utilization: 40, resets_at: 20 },
      },
    }),
    event(2000, 2n, "rate_limit_event", {
      rate_limit_info: {
        rateLimitType: "seven_day",
        utilization: 0.75,
        resetsAt: 20,
      },
    }),
    event(3000, 3n, "get_usage", {
      rate_limits_available: false,
      rate_limits: null,
    }),
  ];
  expect(accountQuota("claude", events, 5000)?.remaining).toBe(20);
  expect(accountQuota("claude", events, 10000)?.remaining).toBe(25);
  expect(accountQuota("claude", events, 20000)).toBeUndefined();
  expect(
    accountQuota("codex", [event(1000, 1n, "turn_end", { totalTokens: 500 })]),
  ).toBeUndefined();
});

it("bounds metadata reads, tolerates one inaccessible session and deduplicates session identities", async () => {
  let active = 0;
  let peak = 0;
  const transcript = vi.fn(
    async (request: { ref: { key: { value: string } }; limit: number }) => {
      peak = Math.max(peak, ++active);
      await new Promise((resolve) => setTimeout(resolve, 1));
      active--;
      if (request.ref.key.value === "inaccessible") throw new Error("Offline");
      return {
        metadata: [event(1000, 1n, "account/rateLimits/updated", codex(90))],
      };
    },
  );
  const sessions = [
    "inaccessible",
    "one",
    "two",
    "three",
    "four",
    "five",
    "one",
    "",
  ].map((runtimeId) => create(SessionSchema, { runtimeId }));
  const c = { sessions: { transcript } } as unknown as Pick<
    Connection,
    "sessions"
  >;
  const quota = await readAccountQuota(
    c,
    "codex",
    sessions,
    new AbortController().signal,
  );
  expect(quota?.remaining).toBe(10);
  expect(peak).toBe(4);
  expect(transcript).toHaveBeenCalledTimes(6);
  expect(transcript.mock.calls.every(([request]) => request.limit === 1)).toBe(
    true,
  );
  const controller = new AbortController();
  controller.abort();
  await expect(
    readAccountQuota(c, "codex", sessions, controller.signal),
  ).rejects.toThrow();
  expect(transcript).toHaveBeenCalledTimes(6);
});
