import { describe, it, expect } from "vitest";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema } from "../gen/cxz/session_pb";
import { mergeEvents, MAX_EVENTS, questions } from "./journal";
const event = (seq: bigint) =>
  create(SessionEventSchema, {
    seq,
    runId: "run",
    kind: "assistant",
    text: "hello",
  });
describe("journal recovery", () => {
  it("deduplicates replayed events without losing uint64 cursor precision", () => {
    const n = 9007199254740993n;
    expect(
      mergeEvents(
        [event(n), event(n + 2n)],
        [event(n + 1n), event(n + 2n)],
      ).map((e) => e.seq),
    ).toEqual([n, n + 1n, n + 2n]);
  });
  it("bounds the rendered history", () => {
    const result = mergeEvents(
      [],
      Array.from({ length: MAX_EVENTS + 100 }, (_, i) => event(BigInt(i))),
    );
    expect(result.length).toBe(MAX_EVENTS);
    expect(result[0].seq).toBe(100n);
  });
  it("keeps native question keys and choices", () => {
    const e = create(SessionEventSchema, {
      text: "AskUserQuestion",
      payload: new TextEncoder().encode(
        JSON.stringify({
          input: {
            questions: [
              {
                question: "Choose?",
                multiSelect: true,
                options: [{ label: "A" }],
              },
            ],
          },
        }),
      ),
    });
    expect(questions("claude", e)[0]).toMatchObject({
      key: "Choose?",
      multi: true,
      other: true,
      options: [{ label: "A" }],
    });
    expect(questions("codex", e)).toEqual([]);
  });
});

import { pendingAfter } from "./journal";
it("reconciles only events newer than the pending snapshot across runs", () => {
  const pending = create(SessionEventSchema, {
    seq: 1n,
    runId: "old",
    requestId: "same",
    kind: "approval",
  });
  const resolved = create(SessionEventSchema, {
    seq: 2n,
    runId: "old",
    requestId: "same",
    kind: "approval_resolved",
  });
  const next = create(SessionEventSchema, {
    seq: 3n,
    runId: "new",
    requestId: "same",
    kind: "approval",
  });
  expect(pendingAfter([pending], [resolved, next], 1n)).toEqual([next]);
  expect(pendingAfter([], [pending, resolved], 2n)).toEqual([]);
});

it("pages toward older history without evicting the newly fetched records", () => {
  const old = Array.from({ length: MAX_EVENTS }, (_, i) =>
    event(BigInt(i + 129)),
  );
  const incoming = Array.from({ length: 129 }, (_, i) => event(BigInt(i + 1)));
  const result = mergeEvents(old, incoming, "older");
  expect(result).toHaveLength(MAX_EVENTS);
  expect(result[0].seq).toBe(1n);
  expect(result.at(-1)!.seq).toBe(BigInt(MAX_EVENTS));
  expect(new Set(result.map((e) => e.seq)).size).toBe(MAX_EVENTS);
});
