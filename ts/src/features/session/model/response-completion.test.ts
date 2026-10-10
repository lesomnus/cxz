import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  SessionEventSchema,
  ResponseMetadataSchema,
} from "../../../../gen/cxz/session_pb";
import { responseCompletions } from "./response-completion";

const event = (seq: bigint, kind: string, text = "") =>
  create(SessionEventSchema, {
    seq,
    kind,
    text,
    runId: "r",
    timeMs: seq * 1000n,
  });
describe("final response summaries", () => {
  it("attaches a durable summary only to its response in the same run", () => {
    const a = event(1n, "assistant"),
      b = event(2n, "assistant"),
      end = event(3n, "turn_end", "completed");
    end.response = create(ResponseMetadataSchema, {
      completionJson: new TextEncoder().encode(
        JSON.stringify({
          response_seq: "2",
          duration_ms: 500,
          duration_source: "provider",
          token_scope: "last_call",
          metrics: { input_tokens: 100, reasoning_tokens: 20, bad: -1 },
        }),
      ),
    });
    const result = responseCompletions([a, b, end]);
    expect(result.size).toBe(1);
    expect(result.get("2")).toEqual({
      durationMs: 500,
      durationSource: "provider",
      tokenScope: "last_call",
      metrics: { input_tokens: 100, reasoning_tokens: 20 },
    });
    end.runId = "other";
    expect(responseCompletions([a, b, end]).size).toBe(0);
  });
  it("legacy commentary, interrupted turns and malformed snapshots are not final", () => {
    const a = event(1n, "assistant"),
      end = event(2n, "turn_end", "completed");
    a.response = create(ResponseMetadataSchema, { phase: "commentary" });
    expect(responseCompletions([a, end]).size).toBe(0);
    a.response.phase = "final_answer";
    end.text = "interrupted";
    expect(responseCompletions([a, end]).size).toBe(0);
    end.text = "completed";
    end.response = create(ResponseMetadataSchema, {
      completionJson: new TextEncoder().encode("invalid"),
    });
    expect(responseCompletions([a, end]).size).toBe(0);
  });
  it("prefers reported legacy duration and never invents missing usage", () => {
    const input = event(1n, "input"),
      a = event(2n, "assistant"),
      end = event(5n, "turn_end", "completed");
    expect(responseCompletions([input, a, end]).get("2")?.durationMs).toBe(
      4000,
    );
    end.payload = new TextEncoder().encode(
      JSON.stringify({ duration_ms: 1234, usage: { output_tokens: 0 } }),
    );
    expect(responseCompletions([input, a, end]).get("2")).toMatchObject({
      durationMs: 1234,
      durationSource: "provider",
      metrics: { output_tokens: 0 },
    });
    expect(
      responseCompletions([a, end]).get("2")?.metrics.input_tokens,
    ).toBeUndefined();
  });
});

it("reads completion snapshots attached to historical responses", () => {
  const answer = event(10n, "assistant");
  answer.response = create(ResponseMetadataSchema, {
    phase: "final_answer",
    completionJson: new TextEncoder().encode(
      JSON.stringify({
        response_seq: "10",
        duration_ms: 800,
        metrics: { input_tokens: 120 },
      }),
    ),
  });
  expect(responseCompletions([answer]).get("10")).toMatchObject({
    durationMs: 800,
    metrics: { input_tokens: 120 },
  });
  answer.response.phase = "commentary";
  expect(responseCompletions([answer]).size).toBe(0);
  answer.response.phase = "final_answer";
  answer.seq = 11n;
  expect(responseCompletions([answer]).size).toBe(0);
});

it("does not reuse a historical final answer for a later empty legacy turn", () => {
  const answer = event(10n, "assistant");
  answer.response = create(ResponseMetadataSchema, {
    completionJson: new TextEncoder().encode(
      JSON.stringify({
        response_seq: "10",
        duration_ms: 800,
        metrics: { input_tokens: 120 },
      }),
    ),
  });
  const end = event(12n, "turn_end", "completed");
  end.payload = new TextEncoder().encode(JSON.stringify({ duration_ms: 1 }));
  expect(responseCompletions([answer, end]).get("10")?.durationMs).toBe(800);
});
