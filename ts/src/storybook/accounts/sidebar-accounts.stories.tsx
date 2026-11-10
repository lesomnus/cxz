import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { SessionSchema } from "#gen/cxz/session_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { SidebarAccounts } from "#src/features/accounts/sidebar-accounts.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { resourceFixture } from "../resource-fixture";
import { projectFixtureId } from "../project-id";

function SidebarPreview({
  noAccounts = false,
  quotaRemaining = 64,
}: {
  noAccounts?: boolean;
  quotaRemaining?: number | null;
}) {
  const [reads, setReads] = useState(0);
  const [c] = useState(() =>
    resourceFixture({
      noAccounts,
      quotaRemaining,
      onRead: () => setReads((value) => value + 1),
    }),
  );
  const [sessions] = useState(() => [
    create(SessionSchema, {
      runtimeId: "demo-review",
      agent: "codex",
      account: { id: projectFixtureId(10) },
    }),
  ]);
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => <>{children}</>,
        view: () => (
          <div
            style={{
              display: "flex",
              height: "90vh",
              width: "min(720px, 95vw)",
            }}
          >
            <nav className="resource-sidebar" aria-label="Resources">
              <span className="brand">cxz</span>
              <SidebarAccounts c={c} sessions={sessions}>
                <RouteLink
                  to="/c/$connection/account"
                  params={{ connection: "_" }}
                  className="resource-link"
                  aria-label="Accounts"
                >
                  <ResourceIcon kind="accounts" />
                </RouteLink>
              </SidebarAccounts>
            </nav>
            <main style={{ padding: "calc(4 * var(--ui-space))", minWidth: 0 }}>
              <p>
                Hover Accounts to see agent logos, account names and remaining
                usage.
              </p>
              <p>
                The preview uses the composer toolbar's shared menu surface.
                Account rows are informational. Escape dismisses it; hovering
                keeps the editor focused.
              </p>
              <textarea
                aria-label="Conversation input"
                placeholder="Keep typing while hovering Accounts"
                style={{ width: "100%" }}
              />
            </main>
          </div>
        ),
      },
      createMemoryHistory({ initialEntries: ["/c/_/account"] }),
    ),
  );
  useEffect(() => () => c.dispose(), [c]);
  return (
    <>
      <RouterProvider router={router} />
      <output aria-label="Account preview reads">{reads}</output>
    </>
  );
}

const meta = {
  title: "Accounts/Sidebar",
  component: SidebarPreview,
  args: { noAccounts: false, quotaRemaining: 64 },
  parameters: {
    docs: {
      description: {
        component:
          "The actual sidebar hover menu. Account metadata and quota snapshots share the Accounts page's connection cache. No account reads occur until the preview opens; unavailable usage stays unknown rather than showing zero. Account rows are informational and have no links. Keyboard focus opens the preview, and Escape dismisses it.",
      },
    },
  },
} satisfies Meta<typeof SidebarPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const UsageUnavailable: Story = { args: { quotaRemaining: null } };
export const Empty: Story = { args: { noAccounts: true } };
