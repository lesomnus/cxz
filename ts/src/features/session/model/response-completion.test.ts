import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import {
  SessionEventSchema,
  ResponseMetadataSchema,
} from "#gen/cxz/session_pb";
import { durationLabel, responseCompletions } from "./response-completion";

const answer = (seq: bigint, completion: unknown) => {
  const e = create(SessionEventSchema, {
    seq,
    kind: "assistant",
    runId: "r",
    timeMs: seq * 1000n,
  });
  e.response = create(ResponseMetadataSchema, {
    phase: "final_answer",
    completionJson: new TextEncoder().encode(
      typeof completion === "string" ? completion : JSON.stringify(completion),
    ),
  });
  return e;
};

// A completed turn's metrics arrive on the response row the runtime folded
// them onto. Which response a turn belongs to, whether its phase makes it
// eligible, and where the duration came from are all decided there -- see
// internal/transcripthistory and cxz#155 -- so this reads rather than derives.
// It used to fold a native turn_end record itself, because a live stream
// carried records while a page carried rows.
it("reads the completion from the row that names itself", () => {
  const mine = answer(10n, {
    response_seq: "10",
    duration_ms: 800,
    duration_source: "provider",
    token_scope: "turn",
    metrics: { input_tokens: 120 },
  });
  expect(responseCompletions([mine]).get("10")).toEqual({
    durationMs: 800,
    durationSource: "provider",
    tokenScope: "turn",
    metrics: { input_tokens: 120 },
  });
});

it("ignores a completion that names another row", () => {
  // The runtime republishes the response it completed, at the sequence of the
  // completing record. A row carrying somebody else's sequence is not a
  // completion of this row, whatever else it is.
  expect(responseCompletions([answer(11n, { response_seq: "10" })]).size).toBe(
    0,
  );
});

it("drops metrics that are not usable numbers, and keeps the answer", () => {
  const e = answer(1n, {
    response_seq: "1",
    duration_ms: -1,
    metrics: { input_tokens: 5, negative: -1, missing: null },
  });
  expect(responseCompletions([e]).get("1")).toMatchObject({
    durationMs: undefined,
    metrics: { input_tokens: 5 },
  });
});

it("keeps an answer whose completion is malformed", () => {
  const rows = [answer(1n, "{not json"), answer(2n, { response_seq: "2" })];
  const found = responseCompletions(rows);
  expect([...found.keys()]).toEqual(["2"]);
});

it("reads a duration as a person would", () => {
  expect(durationLabel(420)).toBe("420ms");
  expect(durationLabel(1500)).toBe("1.5s");
  expect(durationLabel(95000)).toBe("1m 35s");
});
