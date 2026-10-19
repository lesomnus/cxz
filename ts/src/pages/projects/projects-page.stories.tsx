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
  mixedStates = false,
}) {
  const [router] = useState(() => {
    const fixture = projectFixture(unavailable, stopped, sessionCount);
    if (mixedStates) {
      const states = [
        "waiting_input",
        "working",
        "idle",
        "stopped",
        "interrupted",
        "failed",
        "waiting",
        "unknown",
        "running",
      ];
      fixture.sessions.forEach((session, index) => {
        if (session.status) {
          session.status.state = states[index % states.length]!;
          if (session.status.state !== "waiting_input")
            session.status.pending = [];
        }
      });
    }
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
export const SessionStates: Story = {
  args: { sessionCount: 9, mixedStates: true },
};
export const LongSessions: Story = { args: { sessionCount: 16 } };
export const EmptySessions: Story = { args: { sessionCount: 0 } };
export const Empty: Story = { args: { empty: true } };
export const Loading: Story = { args: { empty: true, loading: true } };

export const Unavailable: Story = { args: { unavailable: true } };
export const Stopped: Story = { args: { stopped: true } };

export const LiveInventory: Story = { args: { streaming: true } };
