import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { useWorkspaceRoute, createWorkspaceRouter } from "#src/app/router.tsx";
import { memoryFixture } from "#src/storybook/memory-fixture.ts";
import { memoryLocation } from "#src/shared/navigation/resource-route.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { resourceHandle } from "#src/shared/navigation/resource-route.ts";
import { MemoryPage } from "./memory-page";

// Keep a minimal application navigation link beside the isolated page so its
// document-to-collection flow also works in Storybook.
function MemoryNavigation() {
  const { memoryProject } = useWorkspaceRoute();
  if (!memoryProject) return null;
  return (
    <nav
      aria-label="Resource navigation"
      style={{ position: "absolute", top: 8, left: 0, zIndex: 1 }}
    >
      <RouteLink
        {...memoryLocation(memoryProject)}
        variant="toolbar"
        aria-label="Memory"
        aria-current="page"
        style={{ width: 20, minHeight: 24, padding: 0 }}
      >
        <ResourceIcon kind="memory" />
      </RouteLink>
    </nav>
  );
}

function Preview({
  empty = false,
  fail = false,
  loading = false,
  noSessions = false,
  conflict = false,
  migrated = false,
  cards = false,
  document = false,
} = {}) {
  const [router] = useState(() => {
    const f = memoryFixture({
      empty,
      fail,
      delay: loading ? 60000 : 0,
      conflict,
      migrated,
      cards,
    });
    const router = createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div
            style={{
              display: "grid",
              height: "100vh",
              gridTemplateColumns: "0 minmax(0, 1fr)",
            }}
          >
            <MemoryNavigation />
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
      createMemoryHistory(),
    );
    const first = f.memories
      .filter((m) => projectId(m.project) === projectId(f.projects[0]))
      .sort(
        (a, b) =>
          Number(b.dateUpdated!.seconds - a.dateUpdated!.seconds) ||
          b.dateUpdated!.nanos - a.dateUpdated!.nanos,
      )[0];
    router.history.replace(
      router.buildLocation(
        memoryLocation(
          f.projects[0]!,
          document && first ? resourceHandle(first) : undefined,
        ),
      ).href,
    );
    return router;
  });
  return <RouterProvider router={router} />;
}
const meta = {
  title: "Pages/Memory",
  component: Preview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The collection and document use the same horizontal page transition as Projects and Tasks, below a fixed toolbar. The collection keeps its scroll position and session filter, refreshes after document edits, and stays inert behind the detail. Returning reverses the slide; rapid re-entry starts with saved content. Reduced-motion preferences disable the transition.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Document: Story = { args: { document: true } };
export const Empty: Story = { args: { empty: true } };
export const Unavailable: Story = { args: { fail: true } };
export const Loading: Story = { args: { loading: true } };
export const NoSessions: Story = { args: { noSessions: true } };
export const RevisionConflict: Story = { args: { conflict: true } };

export const MigratedDocuments: Story = { args: { migrated: true } };

export const CardGrid: Story = { args: { cards: true } };
