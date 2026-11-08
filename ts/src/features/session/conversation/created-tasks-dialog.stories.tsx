import { useEffect, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SessionEventSchema, ToolSummarySchema } from "#gen/cxz/session_pb";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { TasksPage } from "#src/pages/tasks/tasks-page.tsx";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import { taskFixture } from "#src/storybook/task-fixture.ts";

function CreatedTasksPreview({
  empty = false,
  unavailable = false,
  delay = 0,
}: {
  empty?: boolean;
  unavailable?: boolean;
  delay?: number;
}) {
  const [fixture] = useState(() => {
    const fixture = taskFixture(18, unavailable, { pageDelay: delay });
    const start = Date.now() - 60_000;
    fixture.tasks.forEach((task, index) => {
      task.dateCreated = timestampFromDate(new Date(start + 4500 + index));
      // The first batch did only reads. Other runs/sessions are never included.
      if (empty || index === 0) task.sourceRun = "another-run";
      if (index === 1) task.sourceSession = "elm";
      if (index === 2)
        task.dateCreated = timestampFromDate(new Date(start - 1000));
    });
    const event = (seq: number, kind: string, text: string) =>
      create(SessionEventSchema, {
        seq: BigInt(seq),
        kind,
        text,
        runId: "oak-run",
        timeMs: BigInt(start + seq * 1000),
      });
    const call = (seq: number, name: string) =>
      create(SessionEventSchema, {
        ...event(seq, "tool_call", name),
        requestId: `task-call-${seq}`,
        toolSummary: create(ToolSummarySchema, {
          name,
          mcpServer: "cxz_tasks",
        }),
      });
    return {
      ...fixture,
      events: [
        event(1, "input", "Create Tasks for the next implementation steps."),
        call(2, "task_list"),
        event(3, "assistant", "I checked the existing work first."),
        call(4, "task_add"),
        call(5, "task_claim"),
        event(
          6,
          "assistant",
          "The Tasks are ready. Open the Task counter to view them.",
        ),
      ],
      taskSource: {
        project: fixture.projects[0],
        session: "oak",
        taskCatalog: fixture.c.taskCatalog,
      },
    };
  });
  useEffect(() => () => fixture.c.dispose(), [fixture]);
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div style={{ height: "100dvh" }}>{children}</div>
        ),
        view: function View() {
          const route = useWorkspaceRoute();
          return route.resource === "tasks" ? (
            <TasksPage
              c={fixture.c}
              projects={fixture.projects}
              sessions={fixture.sessions}
            />
          ) : (
            <ComponentPreview
              events={fixture.events}
              taskSource={fixture.taskSource}
            />
          );
        },
      },
      createMemoryHistory({ initialEntries: ["/c/_/session/oak"] }),
    ),
  );
  return <RouterProvider router={router} />;
}

const meta = {
  title: "Conversation/WorkSummary",
  component: CreatedTasksPreview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The Tasks counter counts MCP calls. Click it to open a small dialog above the button listing Tasks created by this session and run between the surrounding replies. Reads, claims and edits do not create list entries. The first counter is intentionally empty; the second opens a scrollable list with alias-first links to real Task details. Task data reuses the shared project catalog/cache and subscribes only while the dialog is open. Escape or Close restores focus; clicking outside dismisses without changing the transcript layout.",
      },
    },
  },
} satisfies Meta<typeof CreatedTasksPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const CreatedTasks: Story = {};
export const NoCreatedTasks: Story = { args: { empty: true } };
export const LoadingTasks: Story = { args: { delay: 1800 } };
export const UnavailableTasks: Story = { args: { unavailable: true } };
