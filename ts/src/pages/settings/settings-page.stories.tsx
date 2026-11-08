import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Provider } from "@lesomnus/payday/react";
import { create } from "@bufbuild/protobuf";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { Workspace } from "#src/app/workspace-shell.tsx";
import { WorkspaceView } from "#src/app/workspace-view.tsx";
import { projectFixture } from "#src/storybook/project-fixture.ts";
import {
  ProjectListResponseSchema,
  ProjectWatchResponseSchema,
} from "#gen/cxz/project_svc_pb";
import {
  SessionListResponseSchema,
  SessionWatchResponseSchema,
} from "#gen/cxz/session_svc_pb";
import { ConnectionSchema } from "#gen/cxz/connection_pb";
import { ConnectionsPage } from "./connections-page";

const connections = {
  list: async () => ({
    items: [
      create(ConnectionSchema, {
        name: "This server",
        target: "https://cxz.example",
        transport: "connect",
        isDefault: true,
        readOnly: true,
      }),
    ],
  }),
};
function PreviewView() {
  const route = useWorkspaceRoute();
  return route.resource === "settings" &&
    route.settingsTopic === "connections" ? (
    <ConnectionsPage connections={connections} />
  ) : (
    <WorkspaceView />
  );
}
function Preview({
  topic = "general",
}: {
  topic?: "general" | "editor" | "connection";
}) {
  const [fixture] = useState(() => {
    const fixture = projectFixture(false, false, 0);
    const { c, projects } = fixture;
    const idle = (signal?: AbortSignal) =>
      new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener("abort", () => resolve(), { once: true });
      });
    c.projects.list = async () =>
      create(ProjectListResponseSchema, { items: projects });
    c.projects.watch = async function* (_request, options) {
      yield create(ProjectWatchResponseSchema, {
        items: projects.map((value) => ({ id: value.id, value })),
        snapshotComplete: true,
      });
      await idle(options?.signal);
    };
    c.sessions.list = async () => create(SessionListResponseSchema);
    c.sessions.watch = async function* (_request, options) {
      yield create(SessionWatchResponseSchema, { snapshotComplete: true });
      await idle(options?.signal);
    };
    return fixture;
  });
  useEffect(() => () => fixture.c.dispose(), [fixture]);
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <Workspace connection={fixture.c} logout={async () => {}}>
            {children}
          </Workspace>
        ),
        view: PreviewView,
      },
      createMemoryHistory({ initialEntries: [`/setting/${topic}`] }),
    ),
  );
  return (
    <Provider app={fixture.c}>
      <RouterProvider router={router} />
    </Provider>
  );
}
const meta = {
  title: "Pages/Settings",
  component: Preview,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The full workspace with text-only settings navigation, shared resource sections, live browser settings, and the settings.json editor. Wider viewports dock the JSON editor beside the form; smaller viewports use its toolbar toggle. Logs have their own sidebar entry and page.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const General: Story = {};
export const Editor: Story = { args: { topic: "editor" } };
export const Connections: Story = { args: { topic: "connection" } };
