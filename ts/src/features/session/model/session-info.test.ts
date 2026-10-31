import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { SessionSchema, SessionEventSchema } from "#gen/cxz/session_pb";
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
    expect(info.quotas).toMatchObject([
      { key: "codex/primary", remaining: 80, reset: 1900000000000 },
      { key: "codex/secondary", remaining: 10, reset: 1900500000000 },
    ]);
  });
  it("keeps the compact graph empty when only another model's quota bucket is reported", () => {
    const info = sessionInfo(session(), [
      event("usage", "account/rateLimits/updated", {
        rateLimitsByLimitId: { spark: { primary: { usedPercent: 80 } } },
      }),
    ]);
    expect(info.remaining).toBeUndefined();
    expect(info.quotas).toMatchObject([
      { key: "spark/primary", remaining: 20 },
    ]);
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
    expect(info.quotas).toEqual([]);
  });
  it("keeps every Codex bucket for the popover while selecting only the current model's quota", () => {
    const usage = event("usage", "account/rateLimits/updated", {
      rateLimitsByLimitId: {
        codex: {
          primary: { usedPercent: 20, windowDurationMins: 300 },
          secondary: { usedPercent: 60, windowDurationMins: 10080 },
        },
        spark: {
          limitName: "Spark",
          primary: { usedPercent: 95, windowDurationMins: 300 },
        },
      },
    });
    const info = sessionInfo(session(), [usage]);
    expect(info.remaining).toBe(40);
    expect(info.quotas).toMatchObject([
      { key: "codex/primary", bucket: "codex", period: "5h", remaining: 80 },
      {
        key: "codex/secondary",
        bucket: "codex",
        period: "weekly",
        remaining: 40,
      },
      {
        key: "spark/primary",
        bucket: "spark",
        bucketName: "Spark",
        remaining: 5,
      },
    ]);
    const spark = session();
    spark.model = "codex-spark";
    expect(sessionInfo(spark, [usage]).remaining).toBe(5);
  });
  it("replaces only the notified Codex bucket and removes omitted windows from that bucket", () => {
    const info = sessionInfo(session(), [
      event("usage", "account/rateLimits/updated", {
        rateLimitsByLimitId: {
          codex: {
            primary: { usedPercent: 20 },
            secondary: { usedPercent: 60 },
          },
          spark: { primary: { usedPercent: 90 } },
        },
      }),
      event("usage", "account/rateLimits/updated", {
        rateLimits: { limitId: "codex", primary: { usedPercent: 30 } },
      }),
    ]);
    expect(info.quotas?.map((quota) => quota.key)).toEqual([
      "codex/primary",
      "spark/primary",
    ]);
    expect(info.remaining).toBe(70);
  });
  it("retains Claude weekly and model limits when a fractional session event arrives", () => {
    const info = sessionInfo(session("claude"), [
      event("usage", "get_usage", {
        rate_limits: {
          five_hour: { utilization: 10, resets_at: "2030-01-01T00:00:00Z" },
          seven_day: { utilization: 60, resets_at: "2030-01-06T00:00:00Z" },
          seven_day_opus: { utilization: 80 },
          seven_day_sonnet: null,
          model_scoped: [{ display_name: "Fable", utilization: 65 }],
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
    expect(info.quotas).toMatchObject([
      {
        key: "five_hour",
        period: "five_hour",
        remaining: 70,
        reset: 1900000000000,
      },
      {
        key: "seven_day",
        remaining: 40,
        reset: Date.parse("2030-01-06T00:00:00Z"),
      },
      { key: "seven_day_opus", remaining: 20 },
      { key: "model_scoped/0", label: "Fable", remaining: 35 },
    ]);
    expect(info.remaining).toBe(20);
  });
  it("reads both windows from Claude unified notifications without inventing missing utilization", () => {
    const info = sessionInfo(session("claude"), [
      event("usage", "rate_limit_event", {
        rate_limit_info: {
          rateLimitType: "five_hour",
          utilization: 0.2,
          unifiedWindows: {
            five_hour: { utilization: 0.2, resetsAt: 1900000000 },
            seven_day: { status: "allowed" },
          },
        },
      }),
    ]);
    expect(info.quotas).toMatchObject([
      { key: "five_hour", remaining: 80 },
      { key: "seven_day", status: "allowed" },
    ]);
    expect(info.quotas?.[1].remaining).toBeUndefined();
    expect(info.remaining).toBe(80);
  });
  it("merges selected event details with unified status and removes explicitly unavailable windows", () => {
    const info = sessionInfo(session("claude"), [
      event("usage", "get_usage", {
        rate_limits: { seven_day: { utilization: 90 } },
      }),
      event("usage", "rate_limit_event", {
        rate_limit_info: {
          rateLimitType: "five_hour",
          utilization: 0.2,
          resetsAt: 1900000000,
          unifiedWindows: {
            five_hour: { status: "allowed" },
            seven_day: null,
          },
        },
      }),
    ]);
    expect(info.quotas).toMatchObject([
      {
        key: "five_hour",
        remaining: 80,
        status: "allowed",
        reset: 1900000000000,
      },
    ]);
    expect(info.remaining).toBe(80);
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
