import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { ProjectsPage } from "./projects-page";

function Preview({ empty = false, loading = false }) {
  const [router] = useState(() => {
    const projects = empty
      ? []
      : ["Web workspace", "API workspace"].map((name, index) =>
          create(ProjectSchema, {
            id: new Uint8Array([index + 1]),
            runtimeId: `project-${index + 1}`,
            alias: index ? "api" : "web",
            name,
            workspace: "/workspace",
            status: { state: "running" },
          }),
        );
    const sessions = projects.flatMap((project) =>
      ["Plan the next release", "Review changes"].map((name, index) =>
        create(SessionSchema, {
          runtimeId: `${project.alias}-${index}`,
          alias: index ? "elm" : "oak",
          name,
          project,
          agent: index ? "claude" : "codex",
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
              gridTemplateColumns: "0 minmax(0, 1fr)",
            }}
          >
            {children}
          </div>
        ),
        view: () => (
          <ProjectsPage
            projects={projects}
            sessions={sessions}
            projectsLoading={loading}
          />
        ),
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
export const Empty: Story = { args: { empty: true } };
export const Loading: Story = { args: { empty: true, loading: true } };
