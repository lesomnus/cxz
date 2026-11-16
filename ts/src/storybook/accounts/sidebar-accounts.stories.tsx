import { useEffect, useRef, useState } from "react";
import { Button } from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { SessionSchema } from "#gen/cxz/session_pb";
import { SessionTranscriptReplySchema } from "#gen/cxz/session_svc_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { ProfileMenu } from "#src/app/profile-menu.tsx";
import { accountQuotaKey } from "#src/shared/api/read-cache.ts";
import { resourceFixture } from "../resource-fixture";
import { projectFixtureId } from "../project-id";

function SidebarPreview({
  noAccounts = false,
  quotaRemaining = 64,
  quotaDelayMs = 0,
  personalRemaining,
  usageControls = false,
  accountDelayMs = 0,
  singleAccount = false,
}: {
  noAccounts?: boolean;
  quotaRemaining?: number | null;
  quotaDelayMs?: number;
  personalRemaining?: number;
  usageControls?: boolean;
  accountDelayMs?: number;
  singleAccount?: boolean;
}) {
  const quotaValue = useRef(quotaRemaining);
  const [reads, setReads] = useState(0);
  const [c] = useState(() => {
    const c = resourceFixture({
      noAccounts,
      quotaRemaining: () => quotaValue.current,
      onRead: () => setReads((value) => value + 1),
    });
    const transcript = c.sessions.transcript;
    const list = c.accounts.list;
    c.accounts.list = async (request, options) => {
      if (accountDelayMs)
        await new Promise((resolve) => setTimeout(resolve, accountDelayMs));
      options?.signal?.throwIfAborted();
      const page = await list(request, options);
      return singleAccount ? { ...page, items: page.items.slice(0, 1) } : page;
    };
    c.sessions.transcript = async (request, options) => {
      if (
        request.ref?.key?.value === "personal-review" &&
        personalRemaining !== undefined
      ) {
        setReads((value) => value + 1);
        return create(SessionTranscriptReplySchema, {
          metadata: [
            {
              seq: 1n,
              timeMs: BigInt(Date.now()),
              kind: "usage",
              text: "get_usage",
              payload: new TextEncoder().encode(
                JSON.stringify({
                  rate_limits: {
                    five_hour: {
                      utilization: 100 - personalRemaining,
                      resets_at: Math.floor(Date.now() / 1000) + 7200,
                    },
                  },
                }),
              ),
            },
          ],
        });
      }
      if (quotaDelayMs)
        await new Promise((resolve) => setTimeout(resolve, quotaDelayMs));
      options?.signal?.throwIfAborted();
      return transcript(request, options);
    };
    return c;
  });
  const [sessions] = useState(() => [
    create(SessionSchema, {
      runtimeId: "demo-review",
      agent: "codex",
      account: { id: projectFixtureId(10) },
    }),
    ...(personalRemaining === undefined
      ? []
      : [
          create(SessionSchema, {
            runtimeId: "personal-review",
            agent: "claude",
            account: { id: projectFixtureId(11) },
          }),
        ]),
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
              <ProfileMenu
                c={c}
                sessions={sessions}
                logout={async () => {}}
                exitLabel="Sign out"
                openSettings={() => {}}
              />
            </nav>
            <main style={{ padding: "calc(4 * var(--ui-space))", minWidth: 0 }}>
              <p>
                Hover Profile to see agent logos, account names and remaining
                usage, followed by the time until that usage window resets.
              </p>
              <p>
                The preview uses the composer toolbar's shared menu surface.
                Account rows are informational. Escape dismisses it; hovering
                keeps the editor focused.
              </p>
              <textarea
                aria-label="Conversation input"
                placeholder="Keep typing while hovering Profile"
                style={{ width: "100%" }}
              />
              {usageControls && (
                <div
                  style={{
                    display: "flex",
                    gap: "var(--ui-space)",
                    marginTop: "var(--ui-space)",
                    flexWrap: "wrap",
                  }}
                >
                  {[40, 80].map((value) => (
                    <Button
                      key={value}
                      onClick={() => {
                        quotaValue.current = value;
                        c.reads.expire(
                          accountQuotaKey(key(projectFixtureId(10)), "codex", [
                            "demo-review",
                          ]),
                        );
                        window.dispatchEvent(new Event("focus"));
                      }}
                    >
                      Update to {value}%
                    </Button>
                  ))}
                  <Button
                    onClick={() =>
                      c.reads.invalidate(
                        accountQuotaKey(key(projectFixtureId(10)), "codex", [
                          "demo-review",
                        ]),
                      )
                    }
                  >
                    Clear usage cache
                  </Button>
                </div>
              )}
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
          "The actual sidebar hover menu. Account metadata and quota snapshots share the Accounts page's connection cache. No account reads occur until the preview opens; unavailable usage stays unknown rather than showing zero. Each row reserves fixed space for the remaining percentage and its matching usage window's reset countdown, using the Accounts page's day/hour/minute display. Initial gradients enter from left to right with constant acceleration: longer fills take longer, with top-to-bottom starts even for out-of-order responses. Cached values appear immediately, and recent entrances have an account-scoped cooldown. Fresh updates transition the fill from its current position while CompactNumber slides changed digits. Reduced motion shows final values without movement. Account rows are informational and have no links. Keyboard focus opens the preview, and Escape dismisses it.",
      },
    },
  },
} satisfies Meta<typeof SidebarPreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const UsageUnavailable: Story = { args: { quotaRemaining: null } };
export const Empty: Story = { args: { noAccounts: true } };
export const UsageReveal: Story = {
  args: { quotaRemaining: 88, quotaDelayMs: 800, personalRemaining: 28 },
  parameters: {
    docs: {
      description: {
        story:
          "The lower account's usage arrives first. Its light waits for the top account, then follows it; the longer upper fill takes more time to finish. Close and reopen to see cached values immediately.",
      },
    },
  },
};
export const CachedUpdates: Story = {
  args: { quotaRemaining: 64, quotaDelayMs: 200, usageControls: true },
  parameters: {
    docs: {
      description: {
        story:
          "Open once, then reopen to check the immediate cached display. Update to a different quota and reopen: the old cached value appears first, then both the gradient and digits move to the new value. Clearing the usage cache leaves the recent entrance cooldown intact.",
      },
    },
  },
};
export const LoadingAccount: Story = {
  args: { singleAccount: true, accountDelayMs: 350, quotaDelayMs: 500 },
  parameters: {
    docs: {
      description: {
        story:
          "A delayed single-account list and quota show the loading placeholder, usage entrance and final value in the same row height.",
      },
    },
  },
};
