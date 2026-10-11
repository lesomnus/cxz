import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { AuxKind } from "#gen/cxz/project_svc_pb";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import { AuxStateSchema } from "#gen/cxz/session_svc_pb";
import { attachSummaries } from "./session-aux";

function event(seq: number, kind: string, runId = "run") {
  return create(SessionEventSchema, { seq: BigInt(seq), kind, runId });
}

// A live stream keeps the native turn_end record, which is the sequence the
// server names. A projection folds it into the final response, so that
// sequence has no row. Both have to land the summary on the turn's last row.
describe("attaching a summary to the turn it names", () => {
  const summary = (turn: number, text: string, runId = "run") =>
    create(AuxStateSchema, {
      summaries: [{ runId, turn: BigInt(turn), text }],
    });

  it("uses the turn_end row when the stream kept it", () => {
    const events = [
      event(1, "input"),
      event(2, "assistant"),
      event(3, "turn_end"),
    ];
    const got = attachSummaries(events, summary(3, "what happened"));
    expect([...got.keys()]).toEqual(["3"]);
  });

  it("falls back to the last row before the named sequence", () => {
    // The projection dropped turn_end: nothing has sequence 3.
    const events = [event(1, "input"), event(2, "assistant")];
    const got = attachSummaries(events, summary(3, "what happened"));
    expect(got.get("2")?.text).toBe("what happened");
  });

  it("keeps each turn of one run on its own row", () => {
    const events = [
      event(1, "input"),
      event(2, "assistant"),
      event(3, "turn_end"),
      event(4, "input"),
      event(5, "assistant"),
      event(6, "turn_end"),
    ];
    const state = create(AuxStateSchema, {
      summaries: [
        { runId: "run", turn: 3n, text: "first" },
        { runId: "run", turn: 6n, text: "second" },
      ],
    });
    const got = attachSummaries(events, state);
    expect(got.get("3")?.text).toBe("first");
    expect(got.get("6")?.text).toBe("second");
  });

  it("does not borrow a row from another run", () => {
    const events = [event(1, "input", "a"), event(2, "assistant", "a")];
    const got = attachSummaries(events, summary(9, "elsewhere", "b"));
    expect(got.size).toBe(0);
  });

  it("ignores a summary whose turn precedes every loaded row", () => {
    // Older history was trimmed or has not been paged in yet.
    const got = attachSummaries([event(8, "assistant")], summary(3, "gone"));
    expect(got.size).toBe(0);
  });
});

// The task in flight is the newer answer for its turn, and while it runs it is
// the only thing that can say a summary is coming.
describe("the task in flight", () => {
  const events = [event(1, "input"), event(2, "assistant")];
  const running = (over: Record<string, unknown>) =>
    create(AuxStateSchema, {
      summaries: [{ runId: "run", turn: 2n, text: "stored" }],
      current: { runId: "run", turn: 2n, ...over },
    });

  it("replaces a stored summary with its own result", () => {
    const got = attachSummaries(
      events,
      running({
        state: "completed",
        kinds: [AuxKind.SUMMARY],
        results: [{ kind: AuxKind.SUMMARY, text: "fresh" }],
      }),
    );
    expect(got.get("2")).toEqual({
      text: "fresh",
      loading: false,
      failed: false,
    });
  });

  it("says a summary is coming rather than showing the old one", () => {
    const got = attachSummaries(
      events,
      running({ state: "running", kinds: [AuxKind.SUMMARY] }),
    );
    expect(got.get("2")).toEqual({ text: "", loading: true, failed: false });
  });

  it("shows why it failed in the summary's place", () => {
    const got = attachSummaries(
      events,
      running({
        state: "failed",
        kinds: [AuxKind.SUMMARY],
        message: "no account configured",
      }),
    );
    expect(got.get("2")).toEqual({
      text: "no account configured",
      loading: false,
      failed: true,
    });
  });

  it("leaves a stored summary alone when the task is about something else", () => {
    // A title run names the same turn and says nothing about summaries.
    const got = attachSummaries(
      events,
      running({ state: "running", kinds: [AuxKind.TITLE] }),
    );
    expect(got.get("2")?.text).toBe("stored");
  });
});

// responseSeq is the row the manager watched the turn produce. It is used
// directly, so a row between the answer and the turn's end cannot take the
// summary -- which is what the fallback does.
describe("the response the manager named", () => {
  const trailing = [
    create(SessionEventSchema, { seq: 1n, kind: "input", runId: "run" }),
    create(SessionEventSchema, { seq: 2n, kind: "assistant", runId: "run" }),
    create(SessionEventSchema, { seq: 3n, kind: "diagnostic", runId: "run" }),
  ];

  it("lands on the response rather than on what followed it", () => {
    const state = create(AuxStateSchema, {
      summaries: [{ runId: "run", turn: 4n, responseSeq: 2n, text: "note" }],
    });
    expect([...attachSummaries(trailing, state).keys()]).toEqual(["2"]);
  });

  it("without one, falls back and takes the row that followed", () => {
    const state = create(AuxStateSchema, {
      summaries: [{ runId: "run", turn: 4n, text: "note" }],
    });
    // Documented rather than desired: this is the case responseSeq removes.
    expect([...attachSummaries(trailing, state).keys()]).toEqual(["3"]);
  });

  it("shows nothing when the named row has not been paged in", () => {
    const state = create(AuxStateSchema, {
      summaries: [{ runId: "run", turn: 9n, responseSeq: 8n, text: "note" }],
    });
    expect(attachSummaries(trailing, state).size).toBe(0);
  });

  it("uses it for a task in flight too", () => {
    const state = create(AuxStateSchema, {
      current: {
        runId: "run",
        turn: 4n,
        responseSeq: 2n,
        state: "running",
        kinds: [AuxKind.SUMMARY],
      },
    });
    expect(attachSummaries(trailing, state).get("2")?.loading).toBe(true);
  });
});
