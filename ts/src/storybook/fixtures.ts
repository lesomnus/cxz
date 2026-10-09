import { create } from "@bufbuild/protobuf";
import {
  ResponseMetadataSchema,
  SessionEventSchema,
  SessionSchema,
} from "../../gen/cxz/session_pb";
import { ProjectSchema } from "../../gen/cxz/project_pb";

// Synthetic data uses the same protobuf records and projection as the live UI.
export function storyEvent(
  seq: number,
  kind: string,
  text: string,
  value?: unknown,
) {
  return create(SessionEventSchema, {
    seq: BigInt(seq),
    kind,
    text,
    runId: "storybook",
    timeMs: BigInt(Date.now() - 60_000 + seq * 100),
    ...(value
      ? { payload: new TextEncoder().encode(JSON.stringify(value)) }
      : {}),
  });
}
export function responseEvent(
  seq = 2,
  agent = "codex",
  text = "The preview is ready. **No server connection is required.**\n\n- Try editing the message below.\n- Double-click a tool card to inspect its input and output.\n\n[Project documentation](https://github.com/lesomnus/cxz)",
  durationMs = 2400,
) {
  const event = storyEvent(seq, "assistant", text);
  event.response = create(ResponseMetadataSchema, {
    model: agent === "claude" ? "claude-sonnet" : "gpt-6.1",
    effort: "medium",
    modelSource: "response",
    effortSource: "settings",
    phase: "final_answer",
    completionJson: new TextEncoder().encode(
      JSON.stringify({
        response_seq: String(seq),
        duration_ms: durationMs,
        duration_source: "cxz",
        token_scope: "turn",
        metrics: {
          input_tokens: 1240,
          output_tokens: 380,
          cache_read_tokens: 960,
          tool_calls: 1,
        },
      }),
    ),
  });
  return event;
}
export function toolEvents(state = "completed") {
  const command = "/usr/bin/zsh -lc 'git status --short --branch'";
  const call = storyEvent(3, "tool_call", command, {
    item: { type: "commandExecution", command, status: "inProgress" },
  });
  call.requestId = "exec-1";
  if (state === "working") return [call];
  const result = storyEvent(4, "tool_result", "## main\n M ts/src/app.tsx\n", {
    item: {
      type: "commandExecution",
      command,
      status,
      exitCode: state === "completed" ? 0 : 1,
      aggregatedOutput: "## main\n M ts/src/app.tsx\n",
    },
  });
  result.requestId = call.requestId;
  return [call, result];
}
export function fileEvents() {
  const event = storyEvent(3, "tool_result", "Files updated", {
    item: {
      type: "fileChange",
      status: "completed",
      changes: [
        {
          path: "/workspace/ts/src/app.tsx",
          kind: "update",
          diff: "@@ -1,2 +1,3 @@\n-old\n+new\n+line",
        },
        {
          path: "/workspace/ts/src/example.ts",
          kind: "add",
          diff: "@@ -0,0 +1 @@\n+export const ready = true;",
        },
      ],
    },
  });
  event.requestId = "files-1";
  return [event];
}
export function questionEvent() {
  const event = storyEvent(5, "approval", "item/tool/requestUserInput", {
    params: {
      questions: [
        {
          id: "preview",
          question: "Which preview should we build?",
          isOther: true,
          options: [
            {
              label: "Conversation",
              description: "Exercise the editor and message cards together.",
            },
            {
              label: "Settings",
              description: "Check shared controls and editor preferences.",
            },
          ],
        },
      ],
    },
  });
  event.requestId = "question-1";
  return event;
}
export const project = create(ProjectSchema, {
  runtimeId: "storybook",
  alias: "cxz",
  name: "cxz",
});
export function storySession(index = 0) {
  return create(SessionSchema, {
    runtimeId: `session-${index}`,
    alias: ["oak", "seal", "pine"][index % 3],
    name: ["UI exploration", "Editor improvements", "History performance"][
      index % 3
    ],
    agent: index % 3 === 1 ? "claude" : "codex",
    model: "gpt-6.1",
    status: {
      state: index % 3 === 0 ? "working" : index % 3 === 1 ? "idle" : "stopped",
    },
  });
}
export function conversationEvents(agent = "codex", turns = 1) {
  return Array.from({ length: turns }, (_, index) => {
    const seq = index * 2 + 1;
    return [
      storyEvent(
        seq,
        "input",
        `Explore the conversation UI${turns > 1 ? ` · ${index + 1}` : ""}.`,
      ),
      responseEvent(seq + 1, agent),
    ];
  }).flat();
}
