import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  ResponseMetadataSchema,
  SessionEventSchema,
} from "../gen/cxz/session_pb";
import { pendingAfter } from "./journal";
import {
  shellCommand,
  toolLabel,
  toolOutput,
  toolState,
  transcriptEvents,
} from "./tool-activity";

const event = (
  seq: number,
  kind: string,
  requestId: string,
  value: unknown = {},
  text = "",
  runId = "run",
) =>
  create(SessionEventSchema, {
    seq: BigInt(seq),
    kind,
    requestId,
    runId,
    text,
    payload: new TextEncoder().encode(JSON.stringify(value)),
  });
const command = "/usr/bin/zsh -lc 'git status --short --branch'";
const call = event(
  1,
  "tool_call",
  "exec-1",
  { item: { type: "commandExecution", command, status: "inProgress" } },
  command,
);
const approval = event(
  2,
  "approval",
  "214",
  { params: { itemId: "exec-1" } },
  "item/commandExecution/requestApproval",
);
const decision = event(3, "approval_resolved", "214", {}, "allowed");
const result = event(
  5,
  "tool_result",
  "exec-1",
  {
    item: {
      type: "commandExecution",
      command,
      status: "completed",
      exitCode: 0,
      aggregatedOutput: "## branch\n",
    },
  },
  "## branch\n",
);

describe("tool transcript projection", () => {
  it("shows a completed response once while retaining its completion metrics", () => {
    const response = event(1, "assistant", "", {}, "Done");
    const end = event(2, "turn_end", "", {}, "completed");
    end.response = create(ResponseMetadataSchema, {
      completionJson: new TextEncoder().encode(
        JSON.stringify({
          response_seq: "1",
          duration_ms: 500,
          metrics: { output_tokens: 20 },
        }),
      ),
    });
    const history = [response, end];
    const projected = transcriptEvents(history);
    expect(projected.events).toEqual([response]);
    expect(projected.completions.get("1")).toMatchObject({
      durationMs: 500,
      metrics: { output_tokens: 20 },
    });
    expect(history).toEqual([response, end]);
    // A history window lacking the referenced response must keep the notice.
    expect(transcriptEvents([end]).events).toEqual([end]);
    end.runId = "other-run";
    expect(transcriptEvents(history).events).toEqual(history);
  });

  it("keeps failed, interrupted and unmatched completions even after an earlier completed turn in the same run", () => {
    const response = event(1, "assistant", "", {}, "Done");
    const completed = event(2, "turn_end", "", {}, "completed");
    const failed = event(3, "turn_end", "", {}, "failed");
    const interrupted = event(4, "turn_end", "", {}, "interrupted");
    const unmatched = event(5, "turn_end", "", {}, "completed");
    const projected = transcriptEvents([
      response,
      completed,
      failed,
      interrupted,
      unmatched,
    ]);
    expect(projected.events).toEqual([
      response,
      failed,
      interrupted,
      unmatched,
    ]);
    expect(projected.completions.has("1")).toBe(true);
  });

  it("does not hide a completion for commentary or a malformed response snapshot", () => {
    const response = event(1, "assistant", "", {}, "Working");
    response.response = create(ResponseMetadataSchema, { phase: "commentary" });
    const end = event(2, "turn_end", "", {}, "completed");
    expect(transcriptEvents([response, end]).events).toEqual([response, end]);
    response.response.phase = "final_answer";
    end.response = create(ResponseMetadataSchema, {
      completionJson: new TextEncoder().encode("invalid"),
    });
    expect(transcriptEvents([response, end]).events).toEqual([response, end]);
  });

  it("updates one original row from running through approval to completed, retaining the native records", () => {
    const started = transcriptEvents([call]);
    expect(toolState(started.activities.get(1n)!, "codex")).toBe("working");
    const waiting = transcriptEvents([call, approval]);
    expect(waiting.events).toEqual([call]);
    expect(toolState(waiting.activities.get(1n)!, "codex")).toBe("pending");
    expect(pendingAfter([], [call, approval])).toEqual([approval]);
    const allowed = transcriptEvents([call, approval, decision]);
    expect(allowed.events).toEqual([call]);
    expect(toolState(allowed.activities.get(1n)!, "codex")).toBe("working");
    const history = [call, approval, decision, result];
    const finished = transcriptEvents(history);
    expect(finished.events).toEqual([call]);
    const activity = finished.activities.get(1n)!;
    expect(toolState(activity, "codex")).toBe("completed");
    expect(toolLabel(activity, "codex")).toEqual({
      name: "Bash",
      shell: "zsh",
      command: "git status --short --branch",
    });
    expect(activity.approvals[0]).toEqual({
      request: approval,
      resolution: decision,
    });
    expect(activity.result).toBe(result);
    expect(history).toHaveLength(4);
  });

  it("folds interleaved streamed chunks and final output without changing message order or duplicating output", () => {
    const first = event(2, "tool_output", "exec-1", {}, "first\n");
    const second = event(4, "tool_output", "exec-1", {}, "second\n");
    const message = event(3, "assistant", "", {}, "Still working");
    const live = transcriptEvents([call, first, message, second]);
    expect(live.events).toEqual([call, message]);
    expect(toolOutput(live.activities.get(1n)!)).toBe("first\nsecond\n");
    const done = transcriptEvents([call, first, message, second, result]);
    expect(done.events).toEqual([call, message]);
    expect(toolOutput(done.activities.get(1n)!)).toBe("## branch\n");
  });

  it("does not merge identical commands or reused IDs in different runs", () => {
    const sameCommand = event(
      2,
      "tool_call",
      "exec-2",
      { item: { type: "commandExecution", command } },
      command,
    );
    const nextRun = event(
      3,
      "tool_result",
      "exec-1",
      { item: { status: "failed" } },
      "",
      "other-run",
    );
    const projected = transcriptEvents([call, sameCommand, nextRun]);
    expect(projected.events).toEqual([call, sameCommand, nextRun]);
    expect(projected.activities.get(1n)!.result).toBeUndefined();
    expect(toolState(projected.activities.get(3n)!, "codex")).toBe("failed");
  });

  it("keeps orphan output/results at the history edge and pairs them when their call is loaded", () => {
    const chunk = event(4, "tool_output", "exec-1", {}, "## branch\n");
    const edge = transcriptEvents([chunk, result]);
    expect(edge.events).toEqual([chunk]);
    expect(toolState(edge.activities.get(4n)!, "codex")).toBe("completed");
    expect(toolLabel(edge.activities.get(4n)!, "codex").shell).toBe("zsh");
    const expanded = transcriptEvents([call, chunk, result]);
    expect(expanded.events).toEqual([call]);
    expect(transcriptEvents([result]).events).toEqual([result]);
  });

  it("pairs Claude execution approvals and preserves questions and unrelated approval requests", () => {
    const bash = event(1, "tool_call", "bash", { command }, "Bash");
    const permission = event(
      2,
      "approval",
      "request",
      { tool_use_id: "bash" },
      "Bash",
    );
    const question = event(
      3,
      "approval",
      "question",
      { tool_use_id: "bash" },
      "AskUserQuestion",
    );
    const unrelated = event(
      4,
      "approval",
      "unrelated",
      {},
      "Unknown permission",
    );
    const projected = transcriptEvents([bash, permission, question, unrelated]);
    expect(projected.events).toEqual([bash, question, unrelated]);
    expect(toolLabel(projected.activities.get(1n)!, "claude").command).toBe(
      "git status --short --branch",
    );
    expect(
      toolState(transcriptEvents([bash]).activities.get(1n)!, "claude"),
    ).toBe("pending");
    expect(
      pendingAfter([], [bash, permission, question, unrelated]),
    ).toHaveLength(3);
  });

  it("reports nonzero exit codes, declined approvals and Claude error results as failures", () => {
    const failed = event(5, "tool_result", "exec-1", {
      item: { status: "completed", exitCode: 1 },
    });
    expect(
      toolState(transcriptEvents([call, failed]).activities.get(1n)!, "codex"),
    ).toBe("failed");
    const denied = event(3, "approval_resolved", "214", {}, "denied");
    expect(
      toolState(
        transcriptEvents([call, approval, denied]).activities.get(1n)!,
        "codex",
      ),
    ).toBe("failed");
    const claudeError = event(5, "tool_result", "exec-1", {
      is_error: true,
      content: [{ text: "failed to run" }],
    });
    const activity = transcriptEvents([call, claudeError]).activities.get(1n)!;
    expect(toolState(activity, "claude")).toBe("failed");
    expect(toolOutput(activity)).toBe("failed to run");
  });
});

describe("shell display", () => {
  it("unwraps only a single shell script operand and preserves literal quoting and variables", () => {
    expect(shellCommand(command)).toEqual({
      shell: "zsh",
      command: "git status --short --branch",
    });
    expect(shellCommand(`bash -o pipefail -c 'a | b'`)).toEqual({
      shell: "bash",
      command: "a | b",
    });
    expect(shellCommand(`bash --norc -c 'echo $USER'`)).toEqual({
      shell: "bash",
      command: "echo $USER",
    });
    expect(shellCommand(`/bin/sh -c "echo 'hi'; printf \\"%s\\" x"`)).toEqual({
      shell: "sh",
      command: `echo 'hi'; printf "%s" x`,
    });
  });
  it("preserves ambiguous and malformed native commands", () => {
    for (const original of [
      "go test ./...",
      "zsh script.sh",
      "bash --no-rcs script.sh",
      "bash -c 'echo $0' name",
      "python -c 'print(1)'",
      "bash -c ''",
      "bash -c",
      'zsh -lc "unbalanced',
    ]) {
      expect(shellCommand(original)).toEqual({ shell: "", command: original });
    }
  });
});
