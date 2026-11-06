import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { Button } from "@lesomnus/nox";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { TasksPage } from "#src/pages/tasks/tasks-page.tsx";
import { taskFixture } from "#src/storybook/task-fixture.ts";
import styles from "./tasks.module.css";

function taskListPath(path: string) {
  const url = new URL(path, "https://storybook.invalid");
  const project =
    url.pathname.match(/^\/c\/_\/project\/([^/]+)\/task/)?.[1] ??
    url.searchParams.get("project") ??
    "workspace";
  url.searchParams.delete("project");
  return `/c/_/project/${project}/task${url.search}`;
}

function TaskPreview({
  count,
  unavailable = false,
  initialPath = "/c/_/project/workspace/task",
  pageSize = 500,
  pageDelay = 0,
  olderDone = 0,
  longDetails = false,
}: {
  count: number;
  unavailable?: boolean;
  initialPath?: string;
  pageSize?: number;
  pageDelay?: number;
  olderDone?: number;
  longDetails?: boolean;
}) {
  const [fixture] = useState(() =>
    taskFixture(count, unavailable, {
      pageSize,
      pageDelay,
      olderDone,
      longDetails,
    }),
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
        initialEntries:
          initialPath === taskListPath(initialPath)
            ? [initialPath]
            : [taskListPath(initialPath), initialPath],
      }),
    ),
  );
  const [path, setPath] = useState(initialPath);
  useEffect(
    () =>
      router.subscribe("onResolved", () => setPath(router.state.location.href)),
    [router],
  );
  return (
    <>
      <div style={{ height: 36, display: "flex", alignItems: "center" }}>
        <Button type="button" onClick={fixture.agentUpdate}>
          Simulate agent update
        </Button>
        <Button type="button" onClick={() => router.history.back()}>
          Back
        </Button>
        <Button type="button" onClick={() => router.history.forward()}>
          Forward
        </Button>
        <output
          aria-label="Preview route"
          style={{
            font: "10px var(--ui-font-mono)",
            overflow: "hidden",
            whiteSpace: "nowrap",
          }}
        >
          {path}
        </output>
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
          "Project-scoped live task table centered like the Projects and Accounts views. Rows are ordered by creation time, newest first, with Done tasks after every other status. Canceled work stays in the same list. Done tasks updated today or yesterday remain visible; Load more reveals 50 older cached completions at a time, using local calendar days. Open a task to edit, follow its assignee, or explicitly release abandoned work. Columns show status icons, task identity, assignee, priority symbols and relative update times. Priority uses a square, triangle, medium circle, slash and small circle from Highest to Lowest; icon and time popovers provide their full labels and timestamps. Task links nest under their project and prefer aliases with UUID fallback. The project and session selectors share the Memory toolbar layout; session selection filters assigned work locally and changing project clears the filter. The selectors and Add control align with the table; narrow screens scroll the table horizontally. The same connection shares one Watch between consumers. Simulate agent update while editing to preview conflict handling without losing the draft.",
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
export const SessionFilter: Story = {
  args: { initialPath: "/c/_/project/workspace/task?assignee=oak" },
};
export const OlderRuntime: Story = { args: { unavailable: true } };

export const TaskDetails: Story = {
  args: { initialPath: "/c/_/project/workspace/task/task-3" },
  parameters: {
    docs: {
      description: {
        story:
          "A dedicated task detail page using the same Nox SectionGroup and Section surfaces as Projects and Accounts. Metadata and actions occupy the left column; a larger title and description occupy the right. Narrow screens stack the columns. Previous and next task panels peek in from the viewport edges and can be clicked. Wheel or swipe on the page background moves one task per gesture, in table order with the active session/run filter. The metadata scrolls independently and long details scroll within the task. Moving between tasks replaces the current history entry: Back returns to the page before entering details, and Forward restores the last viewed task. Unsaved edits require confirmation before navigation; Add remains a dialog. Alias/UUID deep links keep working.",
      },
    },
  },
};
export const LongTaskDetails: Story = {
  ...TaskDetails,
  args: {
    initialPath: "/c/_/project/workspace/task/task-3",
    longDetails: true,
  },
};
export const SingleTaskDetails: Story = {
  ...TaskDetails,
  args: { count: 1, initialPath: "/c/_/project/workspace/task/task-1" },
};
export const MissingTaskDetails: Story = {
  ...TaskDetails,
  args: { initialPath: "/c/_/project/workspace/task/missing" },
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
