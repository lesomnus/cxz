import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  SessionSchema,
  SessionEventSchema,
} from "../../../../gen/cxz/session_pb";
import { sessionInfo } from "./session-info";

const session = (agent = "codex") =>
  create(SessionSchema, {
    agent,
    model: "test-model",
    status: { runId: "current" },
  });
const event = (kind: string, text: string, value: unknown, runId = "current") =>
  create(SessionEventSchema, {
    kind,
    text,
    runId,
    payload: new TextEncoder().encode(JSON.stringify(value)),
  });

describe("composer session information", () => {
  it("uses the last context snapshot rather than cumulative billing tokens", () => {
    const info = sessionInfo(session(), [
      event("usage", "thread/tokenUsage/updated", {
        tokenUsage: {
          last: { totalTokens: 0 },
          total: { totalTokens: 999999 },
          modelContextWindow: 128000,
        },
      }),
    ]);
    expect(info.contextUsed).toBe(0);
    expect(info.contextWindow).toBe(128000);
    expect(info.remaining).toBeUndefined();
  });
  it("pairs the most constrained quota with its own reset time and handles an explicit unreported effort", () => {
    const info = sessionInfo(session(), [
      event("models", "catalog", {
        model: "selected",
        effective_model: "confirmed",
        effort: "high",
        effective_effort: "",
      }),
      event("usage", "account/rateLimits/updated", {
        rateLimits: {
          primary: { usedPercent: 20, resetsAt: 1900000000 },
          secondary: { usedPercent: 90, resetsAt: 1900500000 },
        },
      }),
    ]);
    expect(info.model).toBe("confirmed");
    expect(info.effort).toBe("");
    expect(info.remaining).toBe(10);
    expect(info.reset).toBe(1900500000000);
  });
  it("does not display a different model's quota bucket", () => {
    const info = sessionInfo(session(), [
      event("usage", "account/rateLimits/updated", {
        rateLimitsByLimitId: { spark: { primary: { usedPercent: 80 } } },
      }),
    ]);
    expect(info.remaining).toBeUndefined();
  });
  it("clears quota when Claude explicitly reports it unavailable", () => {
    const info = sessionInfo(session("claude"), [
      event("usage", "get_usage", {
        rate_limits: { five_hour: { utilization: 50 } },
      }),
      event("usage", "get_usage", {
        rate_limits_available: false,
        rate_limits: null,
      }),
    ]);
    expect(info.remaining).toBeUndefined();
  });
  it("does not keep context from an older run or before compaction", () => {
    const usage = {
      tokenUsage: { last: { totalTokens: 24000 }, modelContextWindow: 128000 },
    };
    expect(
      sessionInfo(session(), [
        event("usage", "thread/tokenUsage/updated", usage, "old"),
      ]).contextUsed,
    ).toBeUndefined();
    expect(
      sessionInfo(session(), [
        event("usage", "thread/tokenUsage/updated", usage),
        event("compact", "", {}),
      ]).contextUsed,
    ).toBeUndefined();
  });
  it("matches Claude's current model capacity and includes input cache tokens", () => {
    const info = sessionInfo(session("claude"), [
      event("turn_end", "", {
        modelUsage: {
          "other-model": { contextWindow: 1000000 },
          "test-model": { contextWindow: 200000 },
        },
      }),
      event("usage", "context/message", {
        model: "test-model",
        usage: {
          input_tokens: 4000,
          cache_read_input_tokens: 20000,
          cache_creation_input_tokens: 0,
        },
      }),
      event("usage", "rate_limit_event", {
        rate_limit_info: {
          rateLimitType: "five_hour",
          utilization: 0.3,
          resetsAt: 1900000000,
        },
      }),
    ]);
    expect(info.contextUsed).toBe(24000);
    expect(info.contextWindow).toBe(200000);
    expect(info.remaining).toBe(70);
  });
  it("ignores malformed payloads and negative counts", () => {
    const broken = create(SessionEventSchema, {
      kind: "usage",
      text: "thread/tokenUsage/updated",
      runId: "current",
      payload: new TextEncoder().encode("null"),
    });
    expect(sessionInfo(session(), [broken]).contextUsed).toBeUndefined();
    expect(
      sessionInfo(session(), [
        event("usage", "thread/tokenUsage/updated", {
          tokenUsage: { last: { totalTokens: -1 }, modelContextWindow: -100 },
        }),
      ]).contextWindow,
    ).toBeUndefined();
  });
});
