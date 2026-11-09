import { useEffect, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Id } from "@lesomnus/payday/pdid";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SessionEventSchema, ToolSummarySchema } from "#gen/cxz/session_pb";
import { SessionRefSchema } from "#gen/cxz/session_svc_pb";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { MemoryPage } from "#src/pages/memory/memory-page.tsx";
import { ComponentPreview } from "#src/storybook/conversation-preview.tsx";
import { memoryFixture } from "#src/storybook/memory-fixture.ts";

function CreatedMemoriesPreview({
  empty = false,
  delay = 0,
  scrollable = false,
}: {
  empty?: boolean;
  delay?: number;
  scrollable?: boolean;
}) {
  const [fixture] = useState(() => {
    const fixture = memoryFixture({ delay, migrated: true });
    const start = Date.now() - 60_000;
    const owner = fixture.sessions[0];
    const ownerId = new Id(owner.id).toString();
    fixture.memories.forEach((memory, index) => {
      memory.name = `Memory note ${index + 1}`;
      memory.dateCreated = timestampFromDate(
        new Date(start + 4500 + index * 100),
      );
      // The API scopes ownership; creation time places each row in its work range.
      memory.ownerSession =
        empty || index === 1
          ? new Id(fixture.sessions[1].id).toString()
          : ownerId;
      if (index === 2)
        memory.dateCreated = timestampFromDate(new Date(start - 1000));
      if (index === 3) memory.dateDeleted = timestampFromDate(new Date());
    });
    const event = (seq: number, kind: string, text: string) =>
      create(SessionEventSchema, {
        seq: BigInt(seq),
        kind,
        text,
        runId: "oak-run",
        timeMs: BigInt(start + seq * 1000),
      });
    const call = (seq: number, name: string) =>
      create(SessionEventSchema, {
        ...event(seq, "tool_call", name),
        requestId: `memory-call-${seq}`,
        toolSummary: create(ToolSummarySchema, {
          name,
          mcpServer: "cxz_memory",
        }),
      });
    return {
      ...fixture,
      events: [
        event(
          1,
          "input",
          scrollable
            ? "Review the implementation and record the decisions.\n\n".repeat(
                40,
              )
            : "Record the implementation decisions in shared memory.",
        ),
        call(2, "memory_list"),
        event(3, "assistant", "I checked the existing notes first."),
        call(4, "memory_add"),
        call(5, "memory_add"),
        call(6, "memory_update"),
        event(
          20,
          "assistant",
          "The notes are ready. Open the Memory counter to view them.",
        ),
      ],
      memorySource: {
        project: fixture.projects[0],
        session: create(SessionRefSchema, {
          key: { case: "id", value: owner.id },
        }),
        memories: fixture.c.memories,
      },
    };
  });
  useEffect(() => () => fixture.dispose(), [fixture]);
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <div style={{ height: "100dvh" }}>{children}</div>
        ),
        view: function View() {
          const route = useWorkspaceRoute();
          return route.resource === "memory" ? (
            <MemoryPage
              c={fixture.c}
              projects={fixture.projects}
              sessions={fixture.sessions}
            />
          ) : (
            <ComponentPreview
              events={fixture.events}
              memorySource={fixture.memorySource}
            />
          );
        },
      },
      createMemoryHistory({ initialEntries: ["/c/_/session/oak"] }),
    ),
  );
  return <RouterProvider router={router} />;
}

const meta = {
  title: "Conversation/WorkSummary/Memory",
  component: CreatedMemoriesPreview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Memory counters open the same anchored menu surface as Tasks. The count and list include active memories created by this session in the response range; reads and edits do not add items. Clicking a note opens its Memory detail page. The menu follows its visible trigger on scroll; Escape restores focus. One session-scoped metadata Watch supplies all ranges without fetching note bodies for the menu.",
      },
    },
  },
} satisfies Meta<typeof CreatedMemoriesPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const CreatedMemories: Story = {};
export const NoCreatedMemories: Story = { args: { empty: true } };
export const LoadingMemories: Story = { args: { delay: 1800 } };
export const ScrollingConversation: Story = { args: { scrollable: true } };
