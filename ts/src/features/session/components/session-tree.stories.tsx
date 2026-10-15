import { useMemo, useState, type PropsWithChildren } from "react";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { SessionTreeGroup } from "./session-tree";
import { ResourcePanel } from "#src/shared/components/resource-panel.tsx";
import { project, storySession } from "#src/storybook/fixtures.ts";
import "#src/storybook/preview.css";

function SessionPanel({
  count = 3,
  loading = false,
  expanded = true,
  unread = false,
  error = false,
  background = false,
  states = [],
}: {
  count?: number;
  loading?: boolean;
  expanded?: boolean;
  unread?: boolean;
  error?: boolean;
  background?: boolean;
  states?: string[];
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
              error={error}
              background={background}
              states={states}
            />
          ),
        },
        createMemoryHistory({ initialEntries: ["/sessions/session-0"] }),
      ),
    [count, loading, expanded, unread, error, background, states],
  );
  return <RouterProvider router={router} />;
}
function SessionList({
  count,
  loading,
  expanded,
  unread,
  error,
  background,
  states,
}: Required<Parameters<typeof SessionPanel>[0]>) {
  const route = useWorkspaceRoute();
  const [open, setOpen] = useState(expanded);
  const items = useMemo(
    () =>
      Array.from({ length: count }, (_, index) => {
        const session = storySession(index);
        if (states[index]) session.status!.state = states[index]!;
        return session;
      }),
    [count, states],
  );
  return (
    <ResourcePanel title="Sessions" className="storybook-panel">
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
          errors={
            error
              ? new Set(
                  [items[1]?.runtimeId].filter((id): id is string => !!id),
                )
              : undefined
          }
          background={
            background ? new Set(items.map((s) => s.runtimeId)) : undefined
          }
          open={open}
          toggle={() => setOpen((value) => !value)}
        />
      </div>
    </ResourcePanel>
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
          "Real project/session rows and panel scrolling, with a memory router. Sessions requiring Resume (stopped, interrupted or failed) have a dimmed identity and an empty activity indicator slot, preserving row alignment. Unread errors remain visible. Idle sessions are ready for input and stay bright. Working dots randomly share the toolbar's green, cyan and violet palette, with a stable color per dot across updates. They grow in their accent color, gradually return to the foreground color, then shrink. Select a session to check highlighting and logo/title/alias press motion. Collapse the project to hide its sessions. The long-list story exposes hover-only scrollbar handles and edge fades.",
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

export const ErrorBeforeCompletion: Story = {
  args: { count: 4, unread: true, error: true },
};

export const BackgroundOnly: Story = { args: { background: true } };
export const BackgroundWithUnread: Story = {
  args: { background: true, unread: true },
};

export const LifecycleStates: Story = {
  args: {
    count: 5,
    states: ["stopped", "interrupted", "failed", "idle", "working"],
    background: true,
    unread: true,
    error: true,
  },
};
