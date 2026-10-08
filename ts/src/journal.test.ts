import { describe, it, expect } from "vitest";
import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionEventSchema } from "../gen/cxz/session_pb";
import { mergeMetadata } from "./session-metadata";
import { sessionInfo } from "./session-info";
import {
  mergeEvents,
  MAX_EVENTS,
  questions,
  isTranscriptEvent,
} from "./journal";
const event = (seq: bigint) =>
  create(SessionEventSchema, {
    seq,
    runId: "run",
    kind: "assistant",
    text: "hello",
  });
describe("transcript events", () => {
  it("keeps protocol and quota polling in history without rendering conversation rows", () => {
    const rows = [
      create(SessionEventSchema, { seq: 1n, kind: "input", text: "Hello" }),
      create(SessionEventSchema, { seq: 2n, kind: "raw" }),
      create(SessionEventSchema, { seq: 3n, kind: "assistant", text: "Hi" }),
      create(SessionEventSchema, {
        seq: 4n,
        kind: "usage_status",
        text: "polling",
      }),
    ];
    const history = mergeEvents([], rows);
    expect(history.filter(isTranscriptEvent).map((e) => e.seq)).toEqual([
      1n,
      3n,
    ]);
    expect(history.map((e) => e.seq)).toEqual([1n, 2n, 3n, 4n]);
  });

  it("still updates the quota indicator from snapshots omitted from the conversation", () => {
    const usage = create(SessionEventSchema, {
      seq: 1n,
      kind: "usage",
      runId: "run",
      text: "account/rateLimits/updated",
      payload: new TextEncoder().encode(
        JSON.stringify({ rateLimits: { primary: { usedPercent: 25 } } }),
      ),
    });
    const history = mergeEvents([], [usage]);
    expect(history.filter(isTranscriptEvent)).toEqual([]);
    const session = create(SessionSchema, {
      agent: "codex",
      status: { runId: "run" },
    });
    expect(sessionInfo(session, mergeMetadata([], history)).remaining).toBe(75);
  });

  it("preserves approval requests and actual diagnostics beside internal failure statuses", () => {
    const rows = [
      create(SessionEventSchema, { kind: "raw" }),
      create(SessionEventSchema, { kind: "usage_status", text: "error" }),
      create(SessionEventSchema, {
        kind: "diagnostic",
        text: "Codex request failed",
      }),
      create(SessionEventSchema, {
        kind: "approval",
        text: "item/commandExecution/requestApproval",
        requestId: "pending",
      }),
      create(SessionEventSchema, { kind: "receipt", text: "send" }),
    ];
    expect(rows.filter(isTranscriptEvent)).toEqual([rows[2], rows[3]]);
  });
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
  it("bounds cached history independently of rendered rows", () => {
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
