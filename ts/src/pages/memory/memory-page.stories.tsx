import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { memoryFixture } from "#src/storybook/memory-fixture.ts";
import { MemoryPage } from "./memory-page";

function Preview({
  empty = false,
  fail = false,
  loading = false,
  noSessions = false,
  conflict = false,
  migrated = false,
} = {}) {
  const [router] = useState(() => {
    const f = memoryFixture({
      empty,
      fail,
      delay: loading ? 60000 : 0,
      conflict,
      migrated,
    });
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
          <MemoryPage
            c={f.c}
            projects={f.projects}
            sessions={noSessions ? [] : f.sessions}
          />
        ),
      },
      createMemoryHistory({ initialEntries: ["/c/_/memory?project=web"] }),
    );
  });
  return <RouterProvider router={router} />;
}
const meta = {
  title: "Pages/Memory",
  component: Preview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Empty: Story = { args: { empty: true } };
export const Unavailable: Story = { args: { fail: true } };
export const Loading: Story = { args: { loading: true } };
export const NoSessions: Story = { args: { noSessions: true } };
export const RevisionConflict: Story = { args: { conflict: true } };

export const MigratedDocuments: Story = { args: { migrated: true } };
