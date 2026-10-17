import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
  ResponseMetadataSchema,
  SessionEventSchema,
} from "#gen/cxz/session_pb";
import { payload } from "./journal";
import {
  shellCommand,
  toolLabel,
  toolOutput,
  toolState,
  toolFiles,
  fileAction,
  fileMeasure,
  transcriptRows,
  activityFromRecords,
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

// Records into rows is Go's now, and its tests moved with it. What is left
// here is this side of the line: indexing the rows a runtime sends, assembling
// one tool's records into the activity a card reads, and what a card displays
// for a given agent. See cxz#155.
describe("row indexing", () => {
  const summarized = (seq: number, state: string, name = "Bash") =>
    create(SessionEventSchema, {
      seq: BigInt(seq),
      kind: "tool_call",
      runId: "run",
      requestId: "exec-1",
      toolSummary: { name, state, command },
    });

  it("finds a tool's activity by the row's own sequence", () => {
    const rows = [summarized(1, "completed"), event(2, "assistant", "")];
    const indexed = transcriptRows(rows);
    expect(indexed.events).toBe(rows);
    expect([...indexed.activities.keys()]).toEqual([1n]);
    expect(indexed.activities.get(1n)!.call).toBe(rows[0]);
    // The records a card needs for output are fetched, not carried.
    expect(indexed.activities.get(1n)!.output).toEqual([]);
    expect(indexed.activities.get(1n)!.result).toBeUndefined();
  });
});

describe("one tool's records", () => {
  it("assembles the call, its output and its result", () => {
    const first = event(2, "tool_output", "exec-1", {}, "first\n");
    const second = event(4, "tool_output", "exec-1", {}, "second\n");
    const activity = activityFromRecords([call, first, second]);
    expect(activity.call).toBe(call);
    expect(toolOutput(activity)).toBe("first\nsecond\n");
    // A result supersedes the chunks it summarizes.
    expect(toolOutput(activityFromRecords([call, first, second, result]))).toBe(
      "## branch\n",
    );
  });

  it("pairs an approval with its resolution and ignores a question", () => {
    const question = event(
      4,
      "approval",
      "question",
      { tool_use_id: "exec-1" },
      "AskUserQuestion",
    );
    const activity = activityFromRecords([call, approval, decision, question]);
    expect(activity.approvals).toEqual([
      { request: approval, resolution: decision },
    ]);
  });

  it("does not pair a resolution that precedes its request", () => {
    const early = event(1, "approval_resolved", "214", {}, "allowed");
    const late = event(2, "approval", "214", { params: { itemId: "exec-1" } });
    expect(activityFromRecords([early, late]).approvals).toEqual([
      { request: late, resolution: undefined },
    ]);
  });
});

describe("what a card displays", () => {
  it("names the tool and the shell it ran under", () => {
    expect(toolLabel(activityFromRecords([call, result]), "codex")).toEqual({
      name: "Bash",
      shell: "zsh",
      command: "git status --short --branch",
    });
  });

  it("reads a nonzero exit, a denial and a Claude error as failures", () => {
    const failed = event(5, "tool_result", "exec-1", {
      item: { status: "completed", exitCode: 1 },
    });
    expect(toolState(activityFromRecords([call, failed]), "codex")).toBe(
      "failed",
    );
    const denied = event(3, "approval_resolved", "214", {}, "denied");
    expect(
      toolState(activityFromRecords([call, approval, denied]), "codex"),
    ).toBe("failed");
    const claudeError = event(5, "tool_result", "exec-1", {
      is_error: true,
      content: [{ text: "failed to run" }],
    });
    const activity = activityFromRecords([call, claudeError]);
    expect(toolState(activity, "claude")).toBe("failed");
    expect(toolOutput(activity)).toBe("failed to run");
  });

  it("holds a call that has reported nothing as pending", () => {
    expect(toolState(activityFromRecords([call]), "claude")).toBe("pending");
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

it("preserves a historical task anchor and applies native live results to it", () => {
  const summary = create(SessionEventSchema, {
    seq: 1n,
    kind: "tool_call",
    requestId: "exec-1",
    runId: "run",
    toolSummary: {
      name: "Bash",
      shell: "zsh",
      command: "git status",
      state: "working",
    },
  });
  // A row that already carries a summary is read from the summary, not from
  // the payload it was reduced from.
  const indexed = transcriptRows([summary]);
  expect([...indexed.activities.keys()]).toEqual([1n]);
  expect(toolState(activityFromRecords([summary]), "codex")).toBe("working");
  expect(toolLabel(activityFromRecords([summary]), "codex").command).toBe(
    "git status",
  );
  expect(toolState(activityFromRecords([summary, result]), "codex")).toBe(
    "completed",
  );
  summary.toolSummary!.state = "failed";
  expect(toolState(activityFromRecords([summary]), "codex")).toBe("failed");
});

it("uses completed Codex file changes at the original anchor, including unknown counts and renames", () => {
  const start = event(1, "tool_call", "files", {
    item: { type: "fileChange", status: "inProgress", changes: [] },
  });
  const end = event(2, "tool_result", "files", {
    item: {
      type: "fileChange",
      status: "completed",
      changes: [
        {
          path: "/workspace/ts/src/app.tsx",
          kind: { type: "update" },
          diff: "--- a/app.tsx\n+++ b/app.tsx\n@@ -1,1 +1,2 @@\n-old\n+new\n+extra\n",
        },
        { path: "/workspace/ts/src/send-motion.ts", kind: { type: "add" } },
        {
          path: "old.ts",
          kind: { type: "update", move_path: "new.ts" },
          diff: "@@ -1 +1 @@\n-a\n+b\n",
        },
      ],
    },
  });
  const activity = activityFromRecords([start, end]);
  expect(toolState(activity, "codex")).toBe("completed");
  const { files } = toolFiles(activity);
  expect(
    files.map((f) => [fileAction(f), f.path, fileMeasure(f), f.movePath]),
  ).toEqual([
    ["Edit", "/workspace/ts/src/app.tsx", "+2 -1", ""],
    ["Write", "/workspace/ts/src/send-motion.ts", "Δ?", ""],
    ["Edit", "old.ts", "+1 -1", "new.ts"],
  ]);
  expect(toolFiles(activityFromRecords([end]))).toEqual({
    files,
    omitted: 0,
  });
  end.requestId = "";
  expect(toolFiles(activityFromRecords([end])).files).toEqual(files);
});

it("distinguishes submitted content/replacement counts from actual diff hunks and preserves unknowns", () => {
  const cases = [
    ["Write", { file_path: "a", content: "one\ntwo\n" }, "+2 content"],
    ["Write", { file_path: "a" }, "Δ?"],
    [
      "Edit",
      {
        input: {
          file_path: "a",
          old_string: "one\ntwo",
          new_string: "new",
          replace_all: true,
        },
      },
      "+1 -2 /match",
    ],
    ["Edit", { file_path: "a", old_string: "", new_string: "" }, "+0 -0"],
    ["Read", { file_path: "a" }, ""],
  ] as const;
  for (const [name, p, expected] of cases) {
    const call = event(1, "tool_call", "file", p, name);
    const file = toolFiles({ call, output: [], approvals: [] }).files[0];
    expect(file.path).toBe("a");
    expect(fileMeasure(file)).toBe(expected);
  }
});

it("renders structured summary files and enriches old summaries from native live completion", () => {
  const call = create(SessionEventSchema, {
    seq: 1n,
    kind: "tool_call",
    requestId: "files",
    runId: "run",
    toolSummary: {
      name: "Files",
      state: "working",
      files: [
        { path: "a", action: "update", added: 2, removed: 1, measure: "diff" },
      ],
      omittedFiles: 3,
    },
  });
  const activity = activityFromRecords([call]);
  expect(fileMeasure(toolFiles(activity).files[0])).toBe("+2 -1");
  expect(toolFiles(activity).omitted).toBe(3);
  call.toolSummary!.files = [];
  const end = event(2, "tool_result", "files", {
    item: {
      type: "fileChange",
      status: "completed",
      changes: [{ path: "a", kind: "delete" }],
    },
  });
  expect(fileAction(toolFiles(activityFromRecords([call, end])).files[0])).toBe(
    "Delete",
  );
});

it("does not interpret result text matching a file tool name as another file request", () => {
  const call = event(
    1,
    "tool_call",
    "file",
    { file_path: "a", content: "one\ntwo\n" },
    "Write",
  );
  const result = event(2, "tool_result", "file", { content: "Write" }, "Write");
  const file = toolFiles(activityFromRecords([call, result])).files[0];
  expect(file.path).toBe("a");
  expect(fileMeasure(file)).toBe("+2 content");
});
