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
}: {
  count: number;
  unavailable?: boolean;
}) {
  const [fixture] = useState(() => taskFixture(count, unavailable));
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
      createMemoryHistory({ initialEntries: ["/tasks?project=project-1"] }),
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
          "Project-scoped live Task board with a shared page toolbar, project icon and compact project selector on every screen size. Open a task to edit, follow its owner, or explicitly release abandoned work. The same connection shares one Watch between consumers. Simulate agent update while editing to preview conflict handling without losing the draft. Narrow boards stack status groups.",
      },
    },
  },
  args: { count: 9 },
} satisfies Meta<typeof TaskPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ProjectBoard: Story = {};
export const ManyTasks: Story = { args: { count: 90 } };
export const EmptyProject: Story = { args: { count: 0 } };
export const OlderRuntime: Story = { args: { unavailable: true } };
