import { useEffect, useState, type PropsWithChildren } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import type { Transport } from "@connectrpc/connect";
import { TaskService } from "#gen/cxz/task_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { sessionUUID } from "#src/shared/api/session-id.ts";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import {
  memoryLocation,
  taskLocation,
} from "#src/shared/navigation/resource-route.ts";
import { useWorkspaceProject } from "#src/app/use-workspace-project.ts";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { TasksPage } from "#src/pages/tasks/tasks-page.tsx";
import { MemoryPage } from "#src/pages/memory/memory-page.tsx";
import { taskFixture } from "./task-fixture";
import { memoryFixture } from "./memory-fixture";

function fixture() {
  const tasks = taskFixture();
  const memories = memoryFixture({ cards: true });
  const sessions = [...tasks.sessions, memories.sessions[2]];
  sessions[2].project = tasks.projects[1];
  for (const memory of memories.memories) {
    const index = memories.sessions.findIndex(
      (session) => sessionUUID(session.id) === memory.ownerSession,
    );
    if (index >= 0) memory.ownerSession = sessionUUID(sessions[index].id);
  }
  const transport: Transport = {
    unary(...args) {
      return (
        args[0].parent.typeName === TaskService.typeName
          ? tasks.c.transport
          : memories.c.transport
      ).unary(...args);
    },
    stream(...args) {
      return (
        args[0].parent.typeName === TaskService.typeName
          ? tasks.c.transport
          : memories.c.transport
      ).stream(...args);
    },
  };
  return {
    c: new Connection("https://shared-scope.invalid", transport),
    projects: tasks.projects,
    sessions,
    dispose() {
      tasks.c.dispose();
      memories.c.dispose();
    },
  };
}

function Preview() {
  const [data] = useState(fixture);
  useEffect(
    () => () => {
      data.c.dispose();
      data.dispose();
    },
    [data],
  );
  const [router] = useState(() => {
    function Shell({ children }: PropsWithChildren) {
      const route = useWorkspaceRoute();
      const project =
        useWorkspaceProject(data.c, data.projects, route) ?? "workspace";
      return (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "0 minmax(0, 1fr)",
            gridTemplateRows: "36px minmax(0, 1fr)",
            height: "100dvh",
          }}
        >
          <nav
            aria-label="Shared scope navigation"
            style={{
              gridColumn: "1 / -1",
              display: "flex",
              alignItems: "center",
            }}
          >
            <RouteLink {...taskLocation({ project })}>Tasks</RouteLink>
            <RouteLink {...memoryLocation(project)}>Memory</RouteLink>
            <RouteLink {...taskLocation({ project: "docs" })}>
              Open Documentation tasks
            </RouteLink>
            <RouteLink
              {...taskLocation({ project: "workspace", assignee: "oak" })}
            >
              Open Task API tasks
            </RouteLink>
          </nav>
          {children}
        </div>
      );
    }
    function View() {
      return useWorkspaceRoute().resource === "memory" ? (
        <MemoryPage {...data} />
      ) : (
        <TasksPage {...data} />
      );
    }
    return createWorkspaceRouter(
      { shell: Shell, view: View },
      createMemoryHistory({ initialEntries: ["/c/_/project/workspace/task"] }),
    );
  });
  return <RouterProvider router={router} />;
}

const meta = {
  title: "Patterns/Resource Selection",
  component: Preview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Project and session selection belong to the Connection. Choose a session on Tasks, open Memory, then return: both use the same scope. Choosing All sessions clears both filters, and choosing another project clears its session. Explicit Task links can override the shared selection. Authenticated browser connections retain the scope in tab-local storage; isolated previews keep it in memory.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SharedScope: Story = {};
