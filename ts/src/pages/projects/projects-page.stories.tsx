import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { projectFixture } from "#src/storybook/project-fixture.ts";
import { ProjectsPage } from "./projects-page";

function Preview({
  empty = false,
  loading = false,
  unavailable = false,
  stopped = false,
  streaming = false,
  sessionCount = 2,
}) {
  const [router] = useState(() => {
    const fixture = projectFixture(
      unavailable,
      stopped,
      streaming ? 4000 : 0,
      sessionCount,
    );
    const projects = empty ? [] : fixture.projects;
    const sessions = empty ? [] : fixture.sessions;
    return createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div
            style={{
              display: "grid",
              height: "100vh",
              gridTemplateColumns: "0 minmax(0, 1fr)",
            }}
          >
            {children}
          </div>
        ),
        view: function DashboardPreview() {
          const [liveSessions, setSessions] = useState(sessions);
          useEffect(() => {
            if (!streaming) return;
            const timer = setInterval(
              () =>
                setSessions((rows) =>
                  rows.map((s) => ({
                    ...s,
                    status: s.status && {
                      ...s.status,
                      lastSeq: s.status.lastSeq + 1n,
                    },
                  })),
                ),
              100,
            );
            return () => clearInterval(timer);
          }, []);
          return (
            <ProjectsPage
              c={fixture.c}
              projects={projects}
              sessions={liveSessions}
              projectsLoading={loading}
            />
          );
        },
      },
      createMemoryHistory({ initialEntries: ["/projects"] }),
    );
  });
  return <RouterProvider router={router} />;
}

const meta = {
  title: "Pages/Projects",
  component: Preview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const LongSessions: Story = { args: { sessionCount: 16 } };
export const EmptySessions: Story = { args: { sessionCount: 0 } };
export const Empty: Story = { args: { empty: true } };
export const Loading: Story = { args: { empty: true, loading: true } };

export const Unavailable: Story = { args: { unavailable: true } };
export const Stopped: Story = { args: { stopped: true } };

export const SlowActivity: Story = { args: { streaming: true } };
