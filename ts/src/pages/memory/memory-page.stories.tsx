import { useEffect, useState } from "react";
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
  readDelay = 0,
  readLoading = false,
  cacheControls = false,
} = {}) {
  const [f] = useState(() =>
    memoryFixture({
      empty,
      fail,
      delay: loading ? 60000 : 0,
      conflict,
      migrated,
      cards,
      readDelay,
      readLoading,
    }),
  );
  useEffect(() => () => f.dispose(), [f]);
  const [router] = useState(() => {
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
            {cacheControls && (
              <button
                type="button"
                style={{ position: "absolute", top: 8, right: 8, zIndex: 10 }}
                onClick={async () => {
                  const memory = f.memories[0];
                  await f.c.memories.patch({
                    ref: {
                      key: { case: "id", value: memory.id },
                      project: {
                        key: { case: "id", value: memory.project!.id },
                      },
                    },
                    content:
                      "# Updated remotely\n\nAnother session changed this document.",
                    baseRevision: memory.revision,
                    dateUpdated: memory.dateUpdated,
                  });
                }}
              >
                Update document on server
              </button>
            )}
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
export const Document: Story = {
  args: { document: true },
  parameters: {
    docs: {
      description: {
        story:
          "Opens the full Memory detail page directly. The Account-style information panel uses shared resource fields in six columns, with three reserved for the inline-editable title. Owner, revision, ID, timestamps, project and labels appear beside it; narrow layouts reduce the column count. Copy, Reload and Delete remain grouped panel actions beside the information panel, followed by the Markdown preview. Copy opens a new draft.",
      },
    },
  },
};
export const DocumentLoading: Story = {
  args: { document: true, readLoading: true },
  parameters: {
    docs: {
      description: {
        story:
          "Opens the same detail route with no cached Read answer. The document Read stays pending so the unloaded metadata and content can be inspected without a timer completing. Leaving the story cancels the fixture request. Document shows the loaded state; Loading shows the collection loading state.",
      },
    },
  },
};
export const Empty: Story = { args: { empty: true } };
export const Unavailable: Story = { args: { fail: true } };
export const Loading: Story = { args: { loading: true } };
export const NoSessions: Story = { args: { noSessions: true } };
export const RevisionConflict: Story = { args: { conflict: true } };

export const MigratedDocuments: Story = { args: { migrated: true } };

export const CardGrid: Story = { args: { cards: true } };

export const CachedDocument: Story = {
  args: { readDelay: 1000, cacheControls: true },
  parameters: {
    docs: {
      description: {
        story:
          "The full Read RPC is deliberately delayed. Open a document, return to the collection, then reopen it: Payday renders its cached answer immediately and revalidates behind it. Update the server document before reopening to verify that an inline title draft survives background refresh and its save preserves the newer Markdown body. This query cache is connection memory; this story does not enable an IndexedDB mirror.",
      },
    },
  },
};
