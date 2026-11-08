import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { clone } from "@bufbuild/protobuf";
import { ContainerState, ProjectSchema } from "#gen/cxz/project_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { projectFixture } from "#src/storybook/project-fixture.ts";
import { projectFixtureId } from "#src/storybook/project-id.ts";
import { ProjectsPage } from "./projects-page";
import { Button } from "@lesomnus/nox";

function Preview({
  list = false,
  projectStates = false,
  empty = false,
  loading = false,
  unavailable = false,
  stopped = false,
  streaming = false,
  sessionCount = 2,
  mixedStates = false,
  longPaths = false,
  homePaths = false,
  failControls = false,
  failErase = false,
  requireTrust = false,
  listActivity = false,
  failProjectControls = false,
  navigation = false,
  failRefresh = false,
  refreshDelayMs = 0,
  usageDelayMs = 0,
}) {
  const [router] = useState(() => {
    const fixture = projectFixture(
      unavailable,
      stopped,
      sessionCount,
      failRefresh
        ? "refresh"
        : failProjectControls
          ? "project-control"
          : failControls
            ? "control"
            : failErase
              ? "erase"
              : requireTrust
                ? "trust"
                : "",
      refreshDelayMs,
    );
    let usageRevision = 1;
    if (usageDelayMs) {
      const readUsage = fixture.c.sessions.conversationStats;
      fixture.c.sessions.conversationStats = async (
        ...args: Parameters<typeof readUsage>
      ) => {
        await new Promise((resolve) => setTimeout(resolve, usageDelayMs));
        const stats = await readUsage(...args);
        if (!args[0].toMs) stats.toMs += BigInt((usageRevision - 1) * 60_000);
        for (const values of [
          stats.total,
          ...stats.days.map((day) => day.values),
        ])
          if (values) values.finishedTurns *= BigInt(usageRevision);
        return stats;
      };
    }
    if (listActivity) {
      const now = Date.now();
      fixture.sessions.forEach((session, index) => {
        const position = index % sessionCount;
        session.name = [
          "Latest conversation",
          "Working conversation",
          "Unread completed conversation",
        ][position]!;
        session.status!.state = position === 1 ? "working" : "idle";
        session.status!.pending = [];
        fixture.activityTimes.set(
          session.runtimeId,
          now - [60_000, 300_000, 1800_000][position]!,
        );
      });
      fixture.notifications.push({
        runtimeId: fixture.sessions[2]!.runtimeId,
        kind: "session.completed",
      });
      // A question is unread too, but does not outrank active work.
      fixture.notifications.push({
        runtimeId: fixture.sessions[3]!.runtimeId,
        kind: "session.question",
      });
    }
    if (projectStates) {
      for (const session of fixture.sessions) {
        session.status!.state = "idle";
        session.status!.pending = [];
      }
      const states = [
        ["Running", ContainerState.RUNNING],
        ["Not started", ContainerState.CREATED],
        ["Stopped", ContainerState.STOPPED],
        ["No container", ContainerState.ABSENT],
        ["Unknown", ContainerState.UNSPECIFIED],
      ] as const;
      const template = fixture.projects[0]!;
      fixture.projects.splice(
        0,
        fixture.projects.length,
        ...states.map(([name, state], index) => {
          const project = clone(ProjectSchema, template);
          project.id = projectFixtureId(index + 1);
          project.alias = `project-${index + 1}`;
          project.name = `${name} workspace`;
          project.status!.containerState = state;
          return project;
        }),
      );
    }
    if (longPaths) {
      fixture.projects[0]!.workspace =
        "/host/" + "long-parent-directory/".repeat(10) + "workspace-leaf";
      fixture.projects[0]!.config =
        ".devcontainer/" + "very-long-leaf-".repeat(12) + ".json";
    }
    if (homePaths) {
      fixture.projects[0]!.workspace =
        "/home/somnus/workspaces/github.com/lesomnus/cxz/.devcontainer/devcontainer.json";
      fixture.projects[0]!.config =
        "/home/somnus/workspaces/github.com/lesomnus/cxz/.devcontainer/" +
        "devcontainer-".repeat(12) +
        ".json";
    }
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
    const history = createMemoryHistory({
      initialEntries: [list || empty ? "/c/_/project" : "/c/_/project/web"],
    });
    return createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div
            style={{
              display: "grid",
              height: "100vh",
              gridTemplateColumns: "0 minmax(0, 1fr)",
              gridTemplateRows:
                navigation || usageDelayMs ? "auto minmax(0, 1fr)" : undefined,
            }}
          >
            {navigation && (
              <nav
                aria-label="Preview history"
                style={{
                  gridColumn: 2,
                  display: "flex",
                  alignItems: "center",
                  gap: "var(--ui-space)",
                  padding: "var(--ui-space)",
                }}
              >
                <Button onClick={() => history.back()}>Back</Button>
                <Button onClick={() => history.forward()}>Forward</Button>
                <span>
                  Open a project, then use Back and Forward to preview both
                  directions.
                </span>
              </nav>
            )}
            {!!usageDelayMs && (
              <nav
                aria-label="Usage preview controls"
                style={{ gridColumn: 2 }}
              >
                <Button onClick={() => usageRevision++}>
                  Change usage on server
                </Button>
              </nav>
            )}
            {children}
          </div>
        ),
        view: function DashboardPreview() {
          const [liveProjects, setProjects] = useState(projects);
          const [liveSessions, setSessions] = useState(sessions);
          useEffect(() => {
            if (listActivity) fixture.c.attention.observe(sessions);
            const stop = fixture.subscribe(() => {
              setProjects([...fixture.projects]);
              setSessions([...fixture.sessions]);
            });
            return () => {
              stop();
              fixture.c.dispose();
            };
          }, []);

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
              projects={liveProjects}
              sessions={liveSessions}
              projectsLoading={loading}
            />
          );
        },
      },
      history,
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
export const Default: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "Dashboard sessions use the project index layout: selection, status, fixed-width session and account links, flexible space, then trailing controls. Resource-link widths share the project index token. Sessions fills the height below Information to align with the actions and usage column, with independent scrolling. Sections use the same tight group spacing and shared column-height tokens as Accounts. Container names retain the installation prefix and abbreviate the storage ID, with in-place expansion and full-name copy. Top-right icon actions use the account action-button style in Erase, Remove, Stop, Start, Refresh order. Actions retain confirmation and state eligibility; Refresh reloads project metadata, inventory, accounts and usage.",
      },
    },
  },
};
export const List: Story = { args: { list: true } };
export const Navigation: Story = {
  args: { list: true, navigation: true },
  parameters: {
    docs: {
      description: {
        story:
          "Open a project from the list: its dashboard slides in from the right below the fixed toolbar, using the shared Tasks and Memory page transition. Back reverses the transition; Forward opens it again. Each page keeps its scroll position and list selection. Direct links start in place, and reduced-motion preferences disable the slide.",
      },
    },
  },
};
export const ListActivity: Story = {
  args: { list: true, listActivity: true, sessionCount: 3 },
};
export const ListBatchPartialFailure: Story = {
  args: { list: true, failProjectControls: true },
};
export const ListStates: Story = { args: { list: true, projectStates: true } };
export const ListRequiresTrust: Story = {
  args: { list: true, stopped: true, requireTrust: true },
};
export const ListUnavailable: Story = {
  args: { list: true, unavailable: true },
};
export const ListLoading: Story = {
  args: { list: true, empty: true, loading: true },
};
export const ListEmpty: Story = { args: { list: true, empty: true } };
export const LongPaths: Story = { args: { longPaths: true } };
export const HomePaths: Story = { args: { homePaths: true } };
export const SessionStates: Story = {
  args: { sessionCount: 9, mixedStates: true },
};
async function selectSessionBatch({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  const table = within(await canvas.findByRole("table", { name: "Sessions" }));
  for (const checkbox of table.getAllByRole("checkbox").slice(1, 5))
    await userEvent.click(checkbox);
  await userEvent.click(
    canvas.getByRole("button", {
      name: "Selected session actions",
    }),
  );
}

export const SessionBatchActions: Story = {
  args: { sessionCount: 6, mixedStates: true },
  play: selectSessionBatch,
  parameters: {
    docs: {
      description: {
        story:
          "Four sessions start selected with the shared batch menu open. The selection includes waiting, working, idle and stopped sessions. Stop skips sessions already stopped; Resume applies to resumable sessions. Stop, Restart and Erase show the shared confirmation dialog. Change the selection and try each action: all session updates use the in-memory project fixture.",
      },
    },
  },
};
export const SessionBatchConfirmation: Story = {
  args: SessionBatchActions.args,
  play: async (context) => {
    await selectSessionBatch(context);
    await userEvent.click(
      within(context.canvasElement.ownerDocument.body).getByRole("menuitem", {
        name: "Stop",
      }),
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          "Starts at the Stop confirmation for the same four-session selection. Only the three eligible sessions appear as targets; the stopped session is skipped. Confirm to update those sessions and clear their selection, or cancel to keep the selection and choose another batch action.",
      },
    },
  },
};
export const LongSessions: Story = { args: { sessionCount: 16 } };
export const EmptySessions: Story = { args: { sessionCount: 0 } };
export const Empty: Story = { args: { empty: true } };
export const Loading: Story = { args: { empty: true, loading: true } };

export const Unavailable: Story = { args: { unavailable: true } };
export const Stopped: Story = { args: { stopped: true } };
export const RequiresTrust: Story = {
  args: { stopped: true, requireTrust: true },
};

export const LiveInventory: Story = { args: { streaming: true } };

export const PartialFailure: Story = { args: { failControls: true } };
export const EraseFailure: Story = { args: { failErase: true } };
export const RefreshFailure: Story = { args: { failRefresh: true } };
export const Refreshing: Story = { args: { refreshDelayMs: 650 } };

export const UsageRefresh: Story = { args: { usageDelayMs: 600 } };
