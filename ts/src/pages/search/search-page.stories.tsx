import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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
  scenario?: "default" | "results" | "empty" | "error" | "slow" | "paginated";
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
            if (scenario === "results") {
              const now = BigInt(Date.now());
              const visits = [
                {
                  sessionId: "session-1",
                  alias: "oak",
                  title: "Search page design",
                  projectName: "Web workspace",
                  agent: "codex",
                  hits: [
                    {
                      seq: 42n,
                      kind: "user",
                      timeMs: now - 60000n,
                      snippet:
                        "Can we keep the search filters compact without changing the conversation view?",
                    },
                    {
                      seq: 43n,
                      kind: "assistant",
                      timeMs: now - 30000n,
                      snippet:
                        "Keep the search bar consistent with conversation search. Project filters belong below the input.",
                    },
                  ],
                },
                {
                  sessionId: "session-2",
                  alias: "birch",
                  title: "API authentication review",
                  projectName: "API workspace",
                  agent: "claude",
                  hits: [
                    {
                      seq: 120n,
                      kind: "assistant",
                      timeMs: now - 3600000n,
                      snippet:
                        "Search checks the retained conversation index before answering. Requests keep the selected project and UTC date boundaries when loading the next page.",
                    },
                    {
                      seq: 123n,
                      kind: "user",
                      timeMs: now - 3540000n,
                      snippet:
                        "Keep authentication errors separate from the search results so the existing matches remain easy to read.",
                    },
                  ],
                },
                {
                  sessionId: "session-3",
                  alias: "cedar",
                  title: "Retention policy follow-up",
                  projectName: "Web workspace",
                  agent: "codex",
                  hits: [
                    {
                      seq: 201n,
                      kind: "assistant",
                      timeMs: now - 86400000n,
                      snippet:
                        "Search returns only retained messages. Older history that has been removed cannot appear in results.",
                    },
                  ],
                },
              ];
              for (const visit of visits)
                yield create(SessionSearchReplySchema, { visit });
              yield create(SessionSearchReplySchema, {
                summary: { hits: 5, sessions: 3, projects: 2 },
              });
              return;
            }
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
export const Results: Story = {
  args: { scenario: "results" },
  parameters: {
    docs: {
      description: {
        story:
          "Opens with a completed search: five user/assistant matches across three sessions and two projects, including longer snippets for checking wrapping.",
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = await canvas.findByRole("searchbox", {
      name: "Search conversations",
    });
    await userEvent.clear(input);
    await userEvent.type(input, "search");
    await userEvent.click(canvas.getByRole("button", { name: "Search" }));
    await canvas.findByText("5 matches · 3 sessions");
  },
};
export const Empty: Story = { args: { scenario: "empty" } };
export const Failure: Story = { args: { scenario: "error" } };
export const Loading: Story = { args: { scenario: "slow" } };
export const Pagination: Story = { args: { scenario: "paginated" } };
