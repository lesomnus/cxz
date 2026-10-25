import { projectFixtureId } from "./project-id";
import { create } from "@bufbuild/protobuf";
import {
  ResponseMetadataSchema,
  SessionEventSchema,
  SessionSchema,
  ToolSummarySchema,
} from "#gen/cxz/session_pb";
import { ProjectSchema } from "#gen/cxz/project_pb";

// Synthetic data is the rows a runtime sends, not the records it reduced them
// from: the app reads rows, so a fixture that emitted records would exercise a
// path it no longer has.
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
  text = "The preview is ready. **No server connection is required.**\n\n- Try editing the message below.\n- Open the task summary to inspect input and output.\n\n[Project documentation](https://github.com/lesomnus/cxz)",
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

// One update per tick, including completions that resize an existing tool row.
export function burstEvent(
  index: number,
  seq: number,
  agent: string,
  turn: string,
) {
  const step = index % 5;
  const batch = Math.floor(index / 5) + 1;
  if (step === 0 || step === 4) {
    const text =
      step === 0
        ? `Update **${batch}**: inspecting the next part of the workspace.`
        : `Update **${batch}**: the check completed.\n\n` +
          (batch % 3 === 0
            ? "```typescript\nexport async function check() {\n  const result = await inspectWorkspace();\n  return { ready: true, result };\n}\n```\n\nThe next check is starting."
            : batch % 3 === 1
              ? "- Read the current state.\n- Compare the expected result.\n- Keep the conversation following new activity.\n\nThis longer reply changes the measured height while more events arrive."
              : "The measured result is ready.\n\n| Check | Result |\n| --- | --- |\n| Workspace | Ready |\n| Conversation | Updated |\n\nContinuing with the next operation.");
    const event = responseEvent(seq, agent, text);
    event.response!.phase = "commentary";
    event.response!.completionJson = new Uint8Array();
    return event;
  }
  const command = `/usr/bin/zsh -lc 'printf "Preview check ${batch}\\n"'`;
  const kind =
    step === 1 ? "tool_call" : step === 2 ? "tool_output" : "tool_result";
  const output =
    `Preview check ${batch}\n` +
    "Workspace check passed.\n".repeat((batch % 4) + 1);
  const event = storyEvent(
    seq,
    kind,
    step === 1 ? (agent === "claude" ? "Bash" : command) : output,
    {
      ...(agent === "claude" ? { command } : {}),
      item: {
        type: "commandExecution",
        command,
        status: step === 3 ? "completed" : "inProgress",
        ...(step === 3 ? { exitCode: 0, aggregatedOutput: output } : {}),
      },
    },
  );
  event.requestId = `${turn}/check-${batch}`;
  return event;
}
// Rows, as a runtime sends them: a tool's call, output and result are already
// one row carrying the summary a card reads. A fixture that emitted the three
// records would be exercising a reduction the app no longer has. See cxz#155.
// Rows, as a runtime sends them: a tool's call, output and result are already
// one row carrying the summary a card reads. A fixture that emitted the three
// records would be exercising a reduction the app no longer has. See cxz#155.
export function toolEvents(state = "completed") {
  const command = "/usr/bin/zsh -lc 'git status --short --branch'";
  const row = storyEvent(3, "tool_call", command, {
    item: { type: "commandExecution", command, status: state },
  });
  row.requestId = "exec-1";
  row.toolSummary = create(ToolSummarySchema, {
    name: "Bash",
    shell: "zsh",
    command: "git status --short --branch",
    state: state === "completed" ? "completed" : state,
  });
  if (state !== "working") row.text = "## main\n M ts/src/app.tsx\n";
  return [row];
}

/**
 * toolRecords is what EventDetails answers with: the records a tool row was
 * reduced from. A row carries no output -- the projection drops output records
 * once the row says what state the tool reached -- so a card that wants the
 * bytes asks for these, and a story that opens one has to be able to answer.
 */
export function toolRecords(state = "completed") {
  const command = "/usr/bin/zsh -lc 'git status --short --branch'";
  const output = "## main\n M ts/src/app.tsx\n";
  const item = { type: "commandExecution", command, status: state };
  const call = storyEvent(3, "tool_call", command, { item });
  const records = [call];
  if (state !== "working") {
    records.push(
      storyEvent(4, "tool_result", output, {
        item: {
          ...item,
          exitCode: state === "completed" ? 0 : 1,
          aggregatedOutput: output,
        },
      }),
    );
  }
  for (const e of records) e.requestId = "exec-1";
  return records;
}

/** loadToolRecords answers any sequence with this tool's records. */
export function loadToolRecords(state = "completed") {
  return async () => toolRecords(state);
}
export function fileEvents() {
  const row = storyEvent(3, "tool_call", "Edit", {
    item: { type: "fileChange", status: "completed" },
  });
  row.requestId = "files-1";
  row.toolSummary = create(ToolSummarySchema, {
    name: "Edit",
    state: "completed",
    files: [
      {
        path: "/workspace/ts/src/app.tsx",
        action: "Edit",
        added: 2,
        removed: 1,
        measure: "lines",
      },
      {
        path: "/workspace/ts/src/example.ts",
        action: "Write",
        added: 1,
        measure: "lines",
      },
    ],
  });
  return [row];
}
export function activityStackEvents() {
  const commands = [
    "git status --short --branch",
    "rg -n 'ActivityCard|ToolActivityView' ts/src/event-view.tsx ts/src/tool-activity-view.tsx ts/src/activity-card.tsx",
    "npm run build && npm run test:storybook -- --grep 'folded activity cards'",
  ];
  // One row per tool, as the projection sends them: its call and its result
  // are the same row by the time a reader has it.
  const tools = commands.map((command, index) => {
    const row = storyEvent(3 + index * 2, "tool_call", "Check passed.", {
      item: {
        type: "commandExecution",
        command,
        status: "completed",
        exitCode: 0,
        aggregatedOutput: "Check passed.",
      },
    });
    row.requestId = `stack-${index}`;
    row.toolSummary = create(ToolSummarySchema, {
      name: "Bash",
      shell: "zsh",
      command,
      state: "completed",
    });
    return row;
  });
  const files = fileEvents()[0];
  files.seq = 9n;
  const introduction = responseEvent(
    2,
    "codex",
    "I’ll inspect the cards, update the files and check the result.",
  );
  introduction.response!.phase = "commentary";
  introduction.response!.completionJson = new Uint8Array();
  return [
    storyEvent(1, "input", "Show the work between conversational responses."),
    introduction,
    ...tools,
    files,
    responseEvent(
      10,
      "codex",
      "The work is summarized. Open the task drawer to inspect the checks and file changes.",
    ),
  ];
}
export type QuestionExample =
  | "choice"
  | "free-text"
  | "steps"
  | "async-steps"
  | "previews"
  | "long";
export function questionEvent(example: QuestionExample = "choice") {
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
  if (example === "free-text" || example === "async-steps") {
    event.text = "agentMessage/questions";
    event.payload = new TextEncoder().encode(
      JSON.stringify({
        item: {
          id: "question-message",
          type: "agentMessage",
          delivery: "async",
          questions:
            example === "free-text"
              ? [
                  {
                    title: "Describe the change you want to make.",
                    options: null,
                  },
                ]
              : [
                  { title: "Choose a theme.", options: ["Light", "Dark"] },
                  { title: "What else should we change?", options: null },
                ],
        },
      }),
    );
  } else if (example === "steps") {
    event.payload = new TextEncoder().encode(
      JSON.stringify({
        params: {
          questions: [
            {
              id: "layout",
              header: "Layout",
              question: "Choose the layout.",
              options: [{ label: "Compact" }, { label: "Spacious" }],
            },
            {
              id: "notes",
              header: "Notes",
              question: "Describe any additional changes.",
              options: null,
            },
            {
              id: "timing",
              header: "Timing",
              question: "When should we ship?",
              options: [{ label: "Now" }, { label: "Later" }],
            },
          ],
        },
      }),
    );
  } else if (example === "long") {
    event.payload = new TextEncoder().encode(
      JSON.stringify({
        params: {
          questions: [
            {
              id: "short",
              isOther: true,
              question: "Choose a layout.",
              options: [{ label: "Compact" }, { label: "Spacious" }],
            },
            {
              id: "long",
              isOther: true,
              question: "Which implementation should we use?",
              options: Array.from({ length: 12 }, (_, index) => ({
                label: `Implementation ${index + 1}`,
                description:
                  "Review the implementation details before choosing.",
                preview:
                  "```ts\n" +
                  "const configuration = { enabled: true };\n".repeat(4) +
                  "```",
              })),
            },
            { id: "notes", question: "Anything else?", options: null },
          ],
        },
      }),
    );
  } else if (example === "previews") {
    event.text = "AskUserQuestion";
    event.payload = new TextEncoder().encode(
      JSON.stringify({
        input: {
          questions: [
            {
              header: "Change",
              question: "Which implementation should we use?",
              options: [
                {
                  label: "Compact",
                  description: "Keep a narrow editor.",
                  preview:
                    "### Compact editor\n\n```css\n.editor { max-width: 600px; }\n```\n\n[Documentation](https://example.com/docs)",
                },
                {
                  label: "Fluid",
                  description: "Use the available width.",
                  preview:
                    "### Fluid editor\n\n```css\n.editor { width: 100%; }\n```",
                },
              ],
            },
          ],
        },
      }),
    );
  }
  event.requestId = "question-1";
  return event;
}
export const project = create(ProjectSchema, {
  id: projectFixtureId(),
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
