import { useState } from "react";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema, ToolSummarySchema } from "#gen/cxz/session_pb";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import {
  activityStackEvents,
  storyEvent,
  toolEvents,
  loadToolRecords,
  responseEvent,
} from "#src/storybook/fixtures.ts";

const meta = {
  title: "Conversation/WorkDialog",
  component: ComponentPreview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Work between conversation replies is summarized in one line: fixed-width total work, cxz_memory and cxz_tasks counts sit on the left in that order, with elapsed time on the right. Counter tooltips align with the trigger's left edge above it and extend to the right. Counts use projected executions, so output/completion updates do not add calls. Zero-count icons stay dim and counts preserve the layout. A + marks an incomplete history range. Elapsed time measures the gap between surrounding replies; the first batch uses the input timestamp. Missing history boundaries leave the duration hidden. View details opens a full-height dialog with outer gaps, sliding from the right inside the conversation. Only the conversation is dimmed; neighboring panes stay uncovered. The list is virtualized and details load on demand. Close with the button, Escape or the dimmed backdrop.",
      },
    },
  },
} satisfies Meta<typeof ComponentPreview>;
export default meta;
type Story = StoryObj<typeof meta>;

const secondTools = toolEvents().map((event, index) => ({
  ...event,
  seq: BigInt(13 + index),
  requestId: "second-turn",
}));
const turns = [
  ...activityStackEvents(),
  storyEvent(12, "input", "Check the next turn separately."),
  ...secondTools,
  responseEvent(15, "codex", "The second check is complete."),
];
export const TurnTasks: Story = { args: { events: turns } };
export const Contained: Story = {
  args: { events: turns },
  decorators: [
    (Story) => (
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "180px minmax(0, 1fr) 240px",
        }}
      >
        <aside className="resource-panel" aria-label="Sessions panel">
          <header>Sessions</header>
          <span>example</span>
        </aside>
        <div style={{ minWidth: 0 }}>
          <Story />
        </div>
        <aside className="file-explorer" aria-label="File explorer">
          <header>Workspace</header>
          <span>README.md</span>
        </aside>
      </div>
    ),
  ],
};
const manyTasks = Array.from({ length: 600 }, (_, index) =>
  toolEvents().map((event, offset) => ({
    ...event,
    seq: BigInt(2 + index * 2 + offset),
    requestId: `task-${index}`,
  })),
).flat();
export const LongTurn: Story = {
  args: {
    events: [
      storyEvent(1, "input", "Inspect this long turn."),
      ...manyTasks,
      responseEvent(1202, "codex", "All checks are complete."),
    ],
  },
};
// One tool row for the turn being read, and an earlier one that paging in
// brings back. A row per tool, so neither needs a result record beside it.
const current = { ...toolEvents()[0], seq: 5n, requestId: "current-task" };
// Stable across renders: paging in an earlier task must not look like the
// selected card being replaced, which is what this story is about.
const earlierTask = { ...toolEvents()[0], seq: 2n, requestId: "earlier-task" };
// Stable too: a fresh loader identity makes useEventDetails refetch, which
// remounts the detail panel and loses the tab the reader had open.
const loadRecords = loadToolRecords();
const final = responseEvent(8, "codex", "The check is complete.");
export const PagedTurn: Story = {
  render: function PagedTurn() {
    const [loaded, setLoaded] = useState(false);
    return (
      <ComponentPreview
        events={
          loaded
            ? [
                storyEvent(1, "input", "Read all work in the turn."),
                earlierTask,
                current,
                final,
              ]
            : [current, final]
        }
        loadDetails={loadRecords}
        workHistory={{
          hasOlder: !loaded,
          loadOlder: async () => setLoaded(true),
        }}
      />
    );
  },
};

const first = toolEvents().map((event, index) => ({
  ...event,
  seq: BigInt(3 + index),
  requestId: "first-batch",
}));
const next = toolEvents().map((event, index) => ({
  ...event,
  seq: BigInt(6 + index),
  requestId: "second-batch",
}));
const more = toolEvents().map((event, index) => ({
  ...event,
  seq: BigInt(8 + index),
  requestId: "third-task",
}));
export const BetweenResponses: Story = {
  args: {
    events: [
      storyEvent(1, "input", "Inspect the two work batches."),
      storyEvent(2, "assistant", "I’ll begin with the first check."),
      ...first,
      storyEvent(
        5,
        "assistant",
        "The first check passed. I’ll do the remaining checks.",
      ),
      ...next,
      ...more,
      responseEvent(10, "codex", "Both batches are complete."),
    ].map((event) => ({
      ...event,
      timeMs: 1710000000000n + event.seq * 2000n,
    })),
  },
};

export const MixedActivity: Story = {
  args: {
    events: [
      storyEvent(
        1,
        "input",
        "Inspect all activity, including non-tool events.",
      ),
      ...toolEvents(),
      storyEvent(5, "compact", "Context compacted"),
      storyEvent(6, "update", "Agent update applied"),
      storyEvent(7, "provider_specific_work", "Provider background work", {
        status: "done",
      }),
      storyEvent(8, "diagnostic", "The workspace connection was interrupted.", {
        error: { message: "The workspace connection was interrupted." },
      }),
      responseEvent(
        9,
        "codex",
        "Work is recorded in the task dialog; the error remains visible.",
      ),
    ],
  },
};

const mcpCall = (seq: number, server: string, tool: string) =>
  create(SessionEventSchema, {
    ...storyEvent(seq, "tool_call", "mcpToolCall"),
    requestId: `mcp-${seq}`,
    toolSummary: create(ToolSummarySchema, {
      name: `${server}.${tool}`,
      mcpServer: server,
      state: "completed",
    }),
  });
export const MemoryAndTaskCalls: Story = {
  args: {
    events: [
      storyEvent(
        1,
        "input",
        "Review shared memory and coordinate the project tasks.",
      ),
      storyEvent(
        2,
        "assistant",
        "I'll read the project notes and check the current tasks.",
      ),
      mcpCall(3, "cxz_memory", "memory_read"),
      mcpCall(4, "cxz_memory", "memory_search"),
      mcpCall(5, "cxz_tasks", "task_list"),
      { ...toolEvents()[0], seq: 6n, requestId: "shell-check" },
      storyEvent(
        7,
        "assistant",
        "The notes are clear. I'll register the next step and update memory.",
      ),
      mcpCall(8, "cxz_memory", "memory_update"),
      mcpCall(9, "cxz_tasks", "task_add"),
      mcpCall(10, "cxz_tasks", "task_claim"),
      responseEvent(
        11,
        "codex",
        "The follow-up work is registered and the shared notes are updated.",
      ),
    ].map((e) => ({ ...e, timeMs: 1710000000000n + e.seq * 2000n })),
  },
};
