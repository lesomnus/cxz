import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { ProjectSchema } from "#gen/cxz/project_pb";
import {
  SessionService,
  SessionSearchReplySchema,
} from "#gen/cxz/session_svc_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { Connection } from "#src/shared/api/connection.ts";
import { projectFixtureId } from "#src/storybook/project-id.ts";
import { SearchPage } from "./search-page";

function Preview({
  scenario = "default",
}: {
  scenario?: "default" | "empty" | "error" | "slow" | "paginated";
}) {
  const [router] = useState(() => {
    const projects = ["Web workspace", "API workspace"].map((name, i) =>
      create(ProjectSchema, {
        id: projectFixtureId(i + 1),
        alias: i ? "api" : "web",
        name,
        workspace: `/workspace/${i ? "api" : "web"}`,
      }),
    );
    const c = new Connection(
      "http://storybook",
      createRouterTransport(({ service }) =>
        service(SessionService, {
          search: async function* (request, context) {
            if (scenario === "error")
              throw new ConnectError(
                "Search is temporarily unavailable",
                Code.Unavailable,
              );
            if (scenario === "slow")
              await new Promise<void>((resolve) => {
                const timer = setTimeout(resolve, 30000);
                context.signal.addEventListener(
                  "abort",
                  () => {
                    clearTimeout(timer);
                    resolve();
                  },
                  { once: true },
                );
              });
            if (context.signal.aborted) return;
            if (scenario !== "empty")
              yield create(SessionSearchReplySchema, {
                visit: {
                  sessionId: "session-1",
                  alias: "oak",
                  title: request.cursor
                    ? "Follow-up review"
                    : "Search page design",
                  projectName: "Web workspace",
                  agent: "codex",
                  hits: [
                    {
                      seq: request.cursor ? 200n : 42n,
                      kind: "assistant",
                      timeMs: 1793851200000n,
                      snippet: request.cursor
                        ? "The follow-up keeps the same project and search options."
                        : "Keep the search bar consistent with conversation search. Project filters belong below the input.",
                    },
                  ],
                },
              });
            yield create(SessionSearchReplySchema, {
              summary: {
                hits: scenario === "empty" ? 0 : 1,
                sessions: scenario === "empty" ? 0 : 1,
                projects: scenario === "empty" ? 0 : 1,
                hasMore: scenario === "paginated" && !request.cursor,
                nextCursor:
                  scenario === "paginated" && !request.cursor ? "page-2" : "",
              },
            });
          },
        }),
      ),
    );
    return createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div
            style={{
              display: "grid",
              height: "100vh",
              gridTemplateColumns: "0 minmax(0,1fr)",
            }}
          >
            {children}
          </div>
        ),
        view: () => <SearchPage c={c} projects={projects} />,
      },
      createMemoryHistory({ initialEntries: ["/c/_/search"] }),
    );
  });
  return <RouterProvider router={router} />;
}
const meta = {
  title: "Pages/Search",
  component: Preview,
  parameters: { layout: "fullscreen" },
  args: { scenario: "default" },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Empty: Story = { args: { scenario: "empty" } };
export const Failure: Story = { args: { scenario: "error" } };
export const Loading: Story = { args: { scenario: "slow" } };
export const Pagination: Story = { args: { scenario: "paginated" } };
