import { useEffect, useState, type ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { clone, create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { ContainerState } from "#gen/cxz/project_pb";
import { ref } from "#src/shared/api/connection.ts";
import {
  SessionSchema,
  SessionEventSchema,
  type Session,
} from "#gen/cxz/session_pb";
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
  SessionReadStateSchema,
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

function History({ children }: { children?: ReactNode }) {
  const router = useRouter();
  return (
    <nav
      aria-label="Preview history"
      style={{ position: "fixed", bottom: 8, right: 8, zIndex: 50 }}
    >
      <Button onClick={() => router.history.back()}>Back</Button>
      <Button onClick={() => router.history.forward()}>Forward</Button>
      {children}
    </nav>
  );
}
function Preview({
  empty = false,
  sessionCount = 5,
  conversation = false,
  recentActivity = false,
  unread = false,
  containerLifecycle = false,
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
    if (containerLifecycle) {
      projects[1]!.status!.containerState = ContainerState.ABSENT;
      projects[1]!.status!.containerId = "";
    }
    if (recentActivity) {
      const now = Date.now();
      sessions.forEach((session, i) => {
        fixture.activityTimes.set(
          session.runtimeId,
          now - (sessions.length - i) * 60000,
        );
      });
      const empty = sessions.at(-1)!;
      fixture.activityTimes.delete(empty.runtimeId);
      empty.dateCreated = timestampFromDate(new Date(now - 30000));
    }
    if (unread) {
      fixture.notifications.push(
        { runtimeId: sessions[0]!.runtimeId, kind: "session.completed" },
        { runtimeId: sessions[2]!.runtimeId, kind: "session.completed" },
      );
    }
    c.sessions.readState = async () =>
      create(SessionReadStateSchema, { generation: "current-run" });
    c.sessions.markRead = async (request) =>
      create(SessionReadStateSchema, {
        generation: "current-run",
        throughSeq: request.throughSeq,
      });
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
      while (!options?.signal?.aborted) {
        await changes(options?.signal);
        if (options?.signal?.aborted) return;
        yield create(ProjectWatchResponseSchema, {
          items: projects.map((value) => ({ id: value.id, value })),
        });
      }
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
            <History>
              {containerLifecycle && (
                <>
                  <Button
                    onClick={() => {
                      void fixture.c.projects.start({
                        ref: {
                          key: { case: "id", value: fixture.projects[1]!.id },
                        },
                      });
                    }}
                  >
                    Start project container
                  </Button>
                  <Button
                    onClick={() => {
                      void fixture.c.projects.stop({
                        ref: {
                          key: { case: "id", value: fixture.projects[1]!.id },
                        },
                      });
                    }}
                  >
                    Stop project container
                  </Button>
                  <Button
                    onClick={() => {
                      void fixture.c.projects.remove({
                        ref: {
                          key: { case: "id", value: fixture.projects[1]!.id },
                        },
                      });
                    }}
                  >
                    Remove project container
                  </Button>
                </>
              )}
              {unread && (
                <Button
                  onClick={() =>
                    fixture.c.attention.setReading(
                      fixture.sessions[0]!.runtimeId,
                      9n,
                    )
                  }
                >
                  Mark first completion read
                </Button>
              )}
              {recentActivity && (
                <>
                  <Button
                    onClick={() => {
                      const session = fixture.sessions[0]!;
                      void fixture.c.sessions.patch({
                        ref: ref(session.runtimeId),
                        name: "Renamed oldest conversation",
                        dateUpdated: session.dateUpdated,
                      });
                    }}
                  >
                    Rename oldest session
                  </Button>
                  <Button
                    onClick={() => {
                      const session = clone(
                        SessionSchema,
                        fixture.sessions[0]!,
                      );
                      fixture.sessions[0] = session;
                      fixture.activityTimes.set(session.runtimeId, Date.now());
                      session.status!.lastSeq += 1n;
                      void fixture.c.sessions.patch({
                        ref: ref(session.runtimeId),
                        name: session.name,
                        dateUpdated: session.dateUpdated,
                      });
                    }}
                  >
                    Record latest conversation
                  </Button>
                </>
              )}
            </History>
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

export const RecentActivity: Story = { args: { recentActivity: true } };

export const UnreadCompletions: Story = { args: { unread: true } };

export const ContainerLifecycle: Story = { args: { containerLifecycle: true } };
