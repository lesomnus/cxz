import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { SessionEventSchema, type Session } from "#gen/cxz/session_pb";
import { Provider } from "@lesomnus/payday/react";
import {
  RouterProvider,
  createMemoryHistory,
  useRouter,
} from "@tanstack/react-router";
import { Button } from "@lesomnus/nox";
import {
  ProjectListResponseSchema,
  ProjectWatchResponseSchema,
} from "#gen/cxz/project_svc_pb";
import {
  SessionListResponseSchema,
  SessionWatchResponseSchema,
  SessionRowSchema,
  SessionEventBatchSchema,
  SessionModelsReplySchema,
  SessionBackgroundReplySchema,
  AuxStateSchema,
} from "#gen/cxz/session_svc_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { Workspace } from "#src/app/workspace-shell.tsx";
import { WorkspaceView } from "#src/app/workspace-view.tsx";
import { sessionFixtureId } from "#src/storybook/fixtures.ts";
import { projectFixture } from "#src/storybook/project-fixture.ts";

function History() {
  const router = useRouter();
  return (
    <nav
      aria-label="Preview history"
      style={{ position: "fixed", bottom: 8, right: 8, zIndex: 50 }}
    >
      <Button onClick={() => router.history.back()}>Back</Button>
      <Button onClick={() => router.history.forward()}>Forward</Button>
    </nav>
  );
}
function Preview({
  empty = false,
  sessionCount = 5,
  conversation = false,
} = {}) {
  const [fixture] = useState(() => {
    const fixture = projectFixture(false, false, empty ? 0 : sessionCount);
    const { c, projects, sessions } = fixture;
    for (const project of projects) project.dateErased = undefined;
    for (const [i, session] of sessions.entries()) {
      session.id = sessionFixtureId(i + 1);
      session.alias = `${session.project!.alias}-${session.alias}`;
      session.status!.pending = [];
      session.status!.state = [
        "idle",
        "working",
        "stopped",
        "waiting_input",
        "failed",
      ][i % 5]!;
    }
    const idle = (signal?: AbortSignal) =>
      new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener("abort", () => resolve(), { once: true });
      });
    const changes = (signal?: AbortSignal) =>
      new Promise<void>((resolve) => {
        const done = () => {
          unsubscribe();
          signal?.removeEventListener("abort", done);
          resolve();
        };
        const unsubscribe = fixture.subscribe(done);
        if (signal?.aborted) done();
        else signal?.addEventListener("abort", done, { once: true });
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
    c.sessions.list = async () =>
      create(SessionListResponseSchema, { items: sessions });
    c.sessions.watch = async function* (_request, options) {
      let previous = new Map(
        sessions.map((session) => [session.runtimeId, session.id]),
      );
      yield create(SessionWatchResponseSchema, {
        items: sessions.map((value) => ({ id: value.id, value })),
        snapshotComplete: true,
      });
      while (!options?.signal?.aborted) {
        await changes(options?.signal);
        if (options?.signal?.aborted) return;
        const current = new Map(
          sessions.map((session) => [session.runtimeId, session.id]),
        );
        const items: { id: Uint8Array; value?: Session }[] = sessions.map(
          (value) => ({ id: value.id, value }),
        );
        for (const [id, resourceID] of previous)
          if (!current.has(id)) items.push({ id: resourceID });
        previous = current;
        yield create(SessionWatchResponseSchema, { items });
      }
    };
    c.sessions.events = async function* (_request, options) {
      yield create(SessionEventSchema, { heartbeat: true });
      await idle(options?.signal);
    };
    c.sessions.history = async () => create(SessionEventBatchSchema);
    c.sessions.models = async () => create(SessionModelsReplySchema);
    c.sessions.background = async () => create(SessionBackgroundReplySchema);
    c.sessions.rows = async function* (_request, options) {
      yield create(SessionRowSchema);
      await idle(options?.signal);
    };
    c.sessions.auxEvents = async function* (_request, options) {
      yield create(AuxStateSchema);
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
            <History />
          </Workspace>
        ),
        view: WorkspaceView,
      },
      createMemoryHistory({
        initialEntries: [
          conversation ? "/c/_/session/web-oak" : "/c/_/session",
        ],
      }),
    ),
  );
  return (
    <Provider app={fixture.c}>
      <RouterProvider router={router} />
    </Provider>
  );
}
const meta = {
  title: "Pages/Sessions",
  component: Preview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The Sessions index reuses the shared overview table and Project-list resource layout. Selection, status, session identity, account/project links and individual/batch controls stay consistent with dashboard tables. Open a session to slide in its navigation panel; use the Sessions sidebar button or preview history to return. The narrow conversation view remains full screen and reduced motion removes the panel transition.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Empty: Story = { args: { empty: true } };
export const ManySessions: Story = { args: { sessionCount: 25 } };
export const Conversation: Story = { args: { conversation: true } };
