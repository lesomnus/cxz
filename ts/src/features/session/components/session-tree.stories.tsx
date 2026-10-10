import { useMemo, useState, type PropsWithChildren } from "react";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { SessionTreeGroup } from "./session-tree";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { project, storySession } from "#src/storybook/fixtures.ts";
import "#src/storybook/preview.css";

function SessionPanel({
  count = 3,
  loading = false,
  expanded = true,
  unread = false,
}: {
  count?: number;
  loading?: boolean;
  expanded?: boolean;
  unread?: boolean;
}) {
  const router = useMemo(
    () =>
      createWorkspaceRouter(
        {
          shell: ({ children }: PropsWithChildren) => <>{children}</>,
          view: () => (
            <SessionList
              count={count}
              loading={loading}
              expanded={expanded}
              unread={unread}
            />
          ),
        },
        createMemoryHistory({ initialEntries: ["/sessions/session-0"] }),
      ),
    [count, loading, expanded, unread],
  );
  return <RouterProvider router={router} />;
}
function SessionList({
  count,
  loading,
  expanded,
  unread,
}: Required<Parameters<typeof SessionPanel>[0]>) {
  const route = useWorkspaceRoute();
  const [open, setOpen] = useState(expanded);
  const items = useMemo(
    () => Array.from({ length: count }, (_, index) => storySession(index)),
    [count],
  );
  return (
    <aside className="resource-panel storybook-panel">
      <header>
        <h2>Sessions</h2>
      </header>
      <PanelScroll>
        <div className="session-tree">
          <SessionTreeGroup
            project={project}
            items={items}
            loading={loading}
            selected={route.session}
            unread={
              unread
                ? new Set(
                    items
                      .filter((s) => s.runtimeId !== route.session)
                      .map((s) => s.runtimeId),
                  )
                : undefined
            }
            open={open}
            toggle={() => setOpen((value) => !value)}
          />
        </div>
      </PanelScroll>
    </aside>
  );
}
const meta = {
  title: "Sessions/Panel",
  component: SessionPanel,
  tags: ["autodocs"],
  args: { count: 3, loading: false, expanded: true },
  parameters: {
    docs: {
      description: {
        component:
          "Real project/session rows and panel scrolling, with a memory router. Select a session to check highlighting and logo/title/alias press motion. Collapse the project to hide its sessions. The long-list story exposes hover-only scrollbar handles and edge fades.",
      },
    },
  },
} satisfies Meta<typeof SessionPanel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const LongList: Story = { args: { count: 24 } };
export const Collapsed: Story = { args: { expanded: false } };
export const Empty: Story = { args: { count: 0 } };
export const Loading: Story = { args: { count: 0, loading: true } };
export const UnreadCompleted: Story = { args: { unread: true } };
