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
  it("keeps messages, approval resolutions and fetched boundaries through protocol floods", () => {
    const input = create(SessionEventSchema, { seq: 1n, kind: "input" });
    const resolved = create(SessionEventSchema, {
      seq: 2n,
      kind: "approval_resolved",
      requestId: "tool",
    });
    const noise = Array.from({ length: MAX_EVENTS * 2 }, (_, i) =>
      create(SessionEventSchema, {
        seq: BigInt(i + 3),
        kind: i % 2 ? "raw" : "usage_status",
      }),
    );
    const result = mergeEvents([input, resolved], noise);
    expect(result).toContain(input);
    expect(result).toContain(resolved);
    expect(result.at(-1)?.seq).toBe(noise.at(-1)?.seq);
    expect(result.length).toBeLessThanOrEqual(MAX_EVENTS);
  });

  it("prepends sparse pages without dropping the latest messages before the pane can fill", () => {
    const newest = event(10000n);
    let cached = [newest];
    for (let page = 0; page < 10; page++) {
      const first = BigInt(10000 - (page + 1) * 128);
      const incoming = Array.from({ length: 128 }, (_, i) =>
        create(SessionEventSchema, {
          seq: first + BigInt(i),
          kind: i === 64 ? "assistant" : "raw",
          text: "recorded",
        }),
      );
      cached = mergeEvents(cached, incoming, "older");
      expect(cached[0].seq).toBe(first);
      expect(cached).toContain(newest);
      expect(cached.filter(isTranscriptEvent)).toHaveLength(page + 2);
      expect(cached.length).toBeLessThanOrEqual(MAX_EVENTS);
    }
  });

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
  it("decodes async free-text and repeated titles with positional reply keys like the TUI", () => {
    const e = create(SessionEventSchema, {
      text: "agentMessage/questions",
      payload: new TextEncoder().encode(
        JSON.stringify({
          item: {
            id: "recorded-message",
            type: "agentMessage",
            delivery: "async",
            questions: [
              { title: "Describe the change.", options: null },
              { title: "Describe the change.", options: ["Light", "Dark"] },
            ],
          },
        }),
      ),
    });
    expect(questions("codex", e)).toEqual([
      {
        key: "0",
        text: "Describe the change.",
        multi: false,
        other: true,
        secret: false,
        options: [],
      },
      {
        key: "1",
        text: "Describe the change.",
        multi: false,
        other: true,
        secret: false,
        options: [{ label: "Light" }, { label: "Dark" }],
      },
    ]);
    expect(questions("claude", e)).toEqual([]);
  });
  it("retains native step headers and option previews without accepting malformed forms", () => {
    const question = {
      id: "step",
      header: "Implementation",
      question: "Choose?",
      options: [
        {
          label: "A",
          description: "Details",
          preview: "```js\nconst a = 1;\n```",
        },
      ],
    };
    const make = (wire: unknown) =>
      create(SessionEventSchema, {
        text: "item/tool/requestUserInput",
        payload: new TextEncoder().encode(
          JSON.stringify({ params: { questions: wire } }),
        ),
      });
    expect(questions("codex", make([question]))[0]).toMatchObject({
      header: "Implementation",
      options: question.options,
    });
    for (const wire of [
      null,
      {},
      [null],
      [{ ...question, id: "" }],
      [question, question],
      [{ ...question, options: [question.options[0], question.options[0]] }],
    ])
      expect(questions("codex", make(wire))).toEqual([]);
    const async = create(SessionEventSchema, {
      text: "agentMessage/questions",
      payload: new TextEncoder().encode(
        JSON.stringify({
          item: {
            id: "q",
            type: "agentMessage",
            delivery: "async",
            questions: [{ title: "Pick", options: ["a", "a"] }],
          },
        }),
      ),
    });
    expect(questions("codex", async)).toEqual([]);
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
