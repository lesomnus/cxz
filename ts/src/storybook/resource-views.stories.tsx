import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AccountsPage } from "#src/pages/accounts/accounts-page.tsx";
import { MemoryLibrary } from "#src/features/memory/memory-library.tsx";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import { BackgroundReport } from "#src/features/activity/background-report.tsx";
import { AuxiliarySettings } from "#src/features/auxiliary/auxiliary-settings.tsx";
import { resourceFixture } from "./resource-fixture";
import { sessionFixtureId } from "./fixtures";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";

function AccountPreview({ c }: { c: Connection }) {
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
                name: "Web account layout",
                alias: "oak",
                account: { alias: "work" },
                agent: "codex",
                status: { state: "idle" },
              }),
              create(SessionSchema, {
                runtimeId: "personal-demo",
                name: "Claude project",
                alias: "elm",
                account: { alias: "personal" },
                agent: "claude",
                status: { state: "stopped" },
              }),
            ]}
          />
        ),
      },
      createMemoryHistory({ initialEntries: ["/accounts"] }),
    ),
  );
  return <RouterProvider router={router} />;
}
function ResourcePreview({
  view,
}: {
  view: "accounts" | "memory" | "usage" | "background" | "auxiliary";
}) {
  const [c] = useState(resourceFixture);
  return (
    <div
      style={{
        width: view === "accounts" ? "min(960px, 100vw)" : "min(600px, 100vw)",
        padding: "var(--space)",
        height: "95vh",
        overflow: view === "accounts" ? "hidden" : "auto",
        display: view === "accounts" ? "flex" : "block",
      }}
    >
      {view === "accounts" ? (
        <AccountPreview c={c} />
      ) : view === "memory" ? (
        <MemoryLibrary c={c} session="demo" />
      ) : view === "usage" ? (
        <UsageReport c={c} />
      ) : view === "background" ? (
        <BackgroundReport c={c} session={sessionFixtureId()} run="run" />
      ) : (
        <AuxiliarySettings c={c} />
      )}
    </div>
  );
}
const meta = {
  title: "Resources/Views",
  component: ResourcePreview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The connected resource components with isolated, typed in-memory RPC fixtures. Provider login needs a real Manager.",
      },
    },
  },
  args: { view: "memory" },
} satisfies Meta<typeof ResourcePreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SharedMemory: Story = {};
export const Accounts: Story = { args: { view: "accounts" } };
export const Usage: Story = { args: { view: "usage" } };
export const BackgroundTasks: Story = { args: { view: "background" } };
export const AuxiliaryAI: Story = { args: { view: "auxiliary" } };
