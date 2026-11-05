import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { Button } from "@lesomnus/nox";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { TasksPage } from "#src/pages/tasks/tasks-page.tsx";
import { taskFixture } from "#src/storybook/task-fixture.ts";
import styles from "./tasks.module.css";

function TaskPreview({
  count,
  unavailable = false,
  initialPath = "/c/_/project/workspace/task",
  pageSize = 500,
  pageDelay = 0,
  olderDone = 0,
}: {
  count: number;
  unavailable?: boolean;
  initialPath?: string;
  pageSize?: number;
  pageDelay?: number;
  olderDone?: number;
}) {
  const [fixture] = useState(() =>
    taskFixture(count, unavailable, { pageSize, pageDelay, olderDone }),
  );
  useEffect(() => () => fixture.c.dispose(), [fixture]);
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div className={styles.preview}>{children}</div>
        ),
        view: () => (
          <TasksPage
            c={fixture.c}
            projects={fixture.projects}
            sessions={fixture.sessions}
          />
        ),
      },
      createMemoryHistory({
        initialEntries: [initialPath],
      }),
    ),
  );
  return (
    <>
      <div style={{ height: 36, display: "flex", alignItems: "center" }}>
        <Button type="button" onClick={fixture.agentUpdate}>
          Simulate agent update
        </Button>
      </div>
      <RouterProvider router={router} />
    </>
  );
}
const meta = {
  title: "Resources/Tasks",
  component: TaskPreview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Project-scoped live task table centered like the Projects and Accounts views. Rows are ordered by creation time, newest first, with Done tasks after every other status. Canceled work stays in the same list. Done tasks updated today or yesterday remain visible; Load more reveals 50 older cached completions at a time, using local calendar days. Open a task to edit, follow its assignee, or explicitly release abandoned work. Columns show status icons, task identity, assignee, priority dots and relative update times. Priority dots vary in size and color; icon and time popovers provide their full labels and timestamps. Task links nest under their project and prefer aliases with UUID fallback. The project selector and Add control align with the table; narrow screens scroll the table horizontally. The same connection shares one Watch between consumers. Simulate agent update while editing to preview conflict handling without losing the draft.",
      },
    },
  },
  args: { count: 9 },
} satisfies Meta<typeof TaskPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ProjectTable: Story = {};
export const ManyTasks: Story = { args: { count: 90 } };
export const CompletedHistory: Story = { args: { olderDone: 125 } };
export const EmptyProject: Story = { args: { count: 0 } };
export const OlderRuntime: Story = { args: { unavailable: true } };

export const TaskDetails: Story = {
  args: { initialPath: "/c/_/project/workspace/task/task-3" },
};
export const TaskById: Story = {
  args: {
    initialPath:
      "/c/_/project/00000000-0000-8000-8007-000000000001/task/00000000-0000-0000-0000-00000000000c",
  },
};

export const ExistingBookmark: Story = {
  args: {
    initialPath: "/c/_/task/task-3?project=workspace&assignee=elm&run=elm-run",
  },
};

export const PagedLoading: Story = {
  args: { count: 12, pageSize: 3, pageDelay: 300 },
  parameters: {
    docs: {
      description: {
        story:
          "The changes-only Watch is ready before List starts. Delayed List pages accumulate offscreen, then all rows appear together in creation order with Done last. Cached rows stay visible during a refresh.",
      },
    },
  },
};
