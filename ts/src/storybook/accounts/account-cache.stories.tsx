import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef, useState } from "react";
import { Button } from "@lesomnus/nox";
import { create } from "@bufbuild/protobuf";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { SessionSchema } from "#gen/cxz/session_pb";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { AccountsPage } from "#src/pages/accounts/accounts-page.tsx";
import { accountRef } from "#src/features/accounts/account-data.ts";
import { resourceFixture } from "../resource-fixture";
import { projectFixtureId } from "../project-id";

function CachePreview({ usageDelayMs = 0 }: { usageDelayMs?: number }) {
  const serverRevision = useRef(1);
  const [visible, setVisible] = useState(true);
  const [serverChanged, setServerChanged] = useState(false);
  const [counts, setCounts] = useState({
    accounts: 0,
    auth: 0,
    usage: 0,
    quota: 0,
  });
  const [c] = useState(() =>
    resourceFixture({
      usageDelayMs,
      usageMultiplier: () => serverRevision.current,
      onRead: (kind) =>
        setCounts((counts) => ({ ...counts, [kind]: counts[kind] + 1 })),
    }),
  );
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => <>{children}</>,
        view: () => (
          <AccountsPage
            c={c}
            sessions={[
              create(SessionSchema, {
                runtimeId: "demo",
                alias: "oak",
                agent: "codex",
                account: { id: projectFixtureId(10) },
                status: { state: "idle" },
              }),
            ]}
          />
        ),
      },
      createMemoryHistory({ initialEntries: ["/c/_/account"] }),
    ),
  );
  return (
    <div
      style={{
        width: "min(960px, 100vw)",
        height: "95vh",
        display: "flex",
        flexDirection: "column",
        gap: 8,
      }}
    >
      <nav
        aria-label="Cache preview controls"
        style={{ display: "flex", flexWrap: "wrap", gap: 8 }}
      >
        <Button onClick={() => setVisible((value) => !value)}>
          {visible ? "Leave accounts" : "Open accounts"}
        </Button>
        <Button
          onClick={async () => {
            const account = await c.accounts.get({ ref: accountRef("work") });
            await c.accounts.patch({
              ref: accountRef("work"),
              dateUpdated: account.dateUpdated,
              name: "Updated account",
            });
            serverRevision.current++;
            setServerChanged(true);
          }}
        >
          Change account on server
        </Button>
        <output aria-label="Server account name">
          {serverChanged ? "Updated account" : "Work"}
        </output>
        {Object.entries(counts).map(([kind, count]) => (
          <output key={kind} aria-label={`${kind} reads`}>
            {kind}: {count}
          </output>
        ))}
      </nav>
      <div style={{ flex: 1, minHeight: 0, display: "flex" }}>
        {visible ? (
          <RouterProvider router={router} />
        ) : (
          <p>
            Accounts view is unmounted. The authenticated connection stays open.
          </p>
        )}
      </div>
    </div>
  );
}

const meta = {
  title: "Accounts/Cache",
  component: CachePreview,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Account metadata and authentication reuse this connection's browser-memory cache for five minutes; statistics, calendar history and quota use one minute. Cached values are read synchronously and appear immediately without animation. Account information and authentication text are always static. Fresh remote numeric results slide through bounded intermediate digits. Leave and reopen to inspect RPC counters. Change account on server simulates an external edit; the square Refresh button after Erase bypasses every account cache. Page reload creates a new connection and clears the cache. No browser disk storage is used.",
      },
    },
  },
} satisfies Meta<typeof CachePreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const SlowRefresh: Story = { args: { usageDelayMs: 600 } };
