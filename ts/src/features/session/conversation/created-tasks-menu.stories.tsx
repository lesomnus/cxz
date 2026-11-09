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
import { projectId } from "#src/shared/api/project-id.ts";

function CreatedTasksPreview({
  empty = false,
  unavailable = false,
  delay = 0,
  cached = false,
  scrollable = false,
}: {
  empty?: boolean;
  unavailable?: boolean;
  delay?: number;
  cached?: boolean;
  scrollable?: boolean;
}) {
  const [fixture] = useState(() => {
    const fixture = taskFixture(18, unavailable, { pageDelay: delay });
    const start = Date.now() - 60_000;
    fixture.tasks.forEach((task, index) => {
      task.dateCreated = timestampFromDate(
        new Date(start + (index + 1) * 1000 + 500),
      );
      // The first batch did only reads. Other runs/sessions are never included.
      if (empty || index === 0) task.sourceRun = "another-run";
      if (index === 1) task.sourceSession = "elm";
      if (index === 2)
        task.dateCreated = timestampFromDate(new Date(start - 1000));
    });
    if (cached)
      fixture.c.taskCatalog
        .forProject(projectId(fixture.projects[0]))
        .apply(fixture.tasks[3]);
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
        event(
          1,
          "input",
          scrollable
            ? "Review the implementation and create Tasks for the next steps.\n\n".repeat(
                40,
              )
            : "Create Tasks for the next implementation steps.",
        ),
        call(2, "task_list"),
        event(3, "assistant", "I checked the existing work first."),
        ...Array.from({ length: 15 }, (_, index) =>
          call(index + 4, "task_add"),
        ),
        call(19, "task_claim"),
        event(
          20,
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
          "The Tasks counter and its menu share the same created-Task list. Reads, claims and edits do not increase the count. Zero-count buttons are disabled; the read-only first batch demonstrates this. The second opens a headerless action-menu list with alias-first links to Task details. Rows, text placement, surface and scrollbar reuse Nox menu primitives; height follows content up to the shared menu limit. A single shared project catalog/cache subscription updates both count and list, including while the menu is open. Loading starts disabled until data is available; Refreshing keeps cached Tasks available during the refresh. An open trigger retains its hover background. The menu follows conversation scrolling and stays open while its trigger remains visible; scrolling inside its list is independent. Escape restores focus; clicking outside dismisses without changing transcript layout.",
      },
    },
  },
} satisfies Meta<typeof CreatedTasksPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const CreatedTasks: Story = {};
export const ScrollingConversation: Story = { args: { scrollable: true } };
export const NoCreatedTasks: Story = { args: { empty: true } };
export const LoadingTasks: Story = { args: { delay: 1800 } };
export const RefreshingTasks: Story = { args: { delay: 1800, cached: true } };
export const UnavailableTasks: Story = {
  args: { unavailable: true, cached: true },
};
