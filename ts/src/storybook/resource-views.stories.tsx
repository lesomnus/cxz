import { Id } from "@lesomnus/payday/pdid";
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AccountsPage } from "#src/pages/accounts/accounts-page.tsx";
import { memoryFixture } from "./memory-fixture";
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
import { ProjectSchema } from "#gen/cxz/project_pb";
import { projectFixtureId } from "./project-id";
import type { Connection } from "#src/shared/api/connection.ts";

function AccountPreview({
  c,
  extraSessions = 0,
}: {
  c: Connection;
  extraSessions?: number;
}) {
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => <>{children}</>,
        view: () => (
          <AccountsPage
            c={c}
            projects={[
              create(ProjectSchema, {
                id: projectFixtureId(1),
                alias: "web",
                name: "Web workspace",
              }),
              create(ProjectSchema, {
                id: projectFixtureId(2),
                alias: "api",
                name: "API workspace",
              }),
            ]}
            sessions={[
              create(SessionSchema, {
                runtimeId: "demo",
                name: "Web account layout",
                alias: "oak",
                project: { id: projectFixtureId(1) },
                account: { id: projectFixtureId(10) },
                agent: "codex",
                status: { state: "idle" },
              }),
              create(SessionSchema, {
                runtimeId: "personal-demo",
                name: "Claude project",
                alias: "elm",
                project: { id: projectFixtureId(2) },
                account: { id: projectFixtureId(11) },
                agent: "claude",
                status: { state: "stopped" },
              }),
              create(SessionSchema, {
                runtimeId: "demo-review",
                name: "Review deployment changes",
                alias: "pine",
                project: { id: projectFixtureId(2) },
                account: { id: projectFixtureId(10) },
                agent: "codex",
                status: { state: "working" },
              }),
              ...Array.from({ length: extraSessions }, (_, index) =>
                create(SessionSchema, {
                  id: projectFixtureId(index + 100),
                  runtimeId: `extra-${index}`,
                  alias: `extra-${index}`,
                  name: `Extra session ${index + 1}`,
                  project: { id: projectFixtureId(1) },
                  account: { id: projectFixtureId(10) },
                  agent: "codex",
                  status: { state: "idle" },
                }),
              ),
            ]}
          />
        ),
      },
      createMemoryHistory({ initialEntries: ["/c/_/account"] }),
    ),
  );
  return <RouterProvider router={router} />;
}
function ResourcePreview({
  view,
  eraseFailures = 0,
  usageDelayMs = 0,
  usageFailures = 0,
  longAccountDetails = false,
  editFailures = 0,
  extraSessions = 0,
  quotaRemaining = 64,
}: {
  view: "accounts" | "memory" | "usage" | "background" | "auxiliary";
  eraseFailures?: number;
  usageDelayMs?: number;
  usageFailures?: number;
  longAccountDetails?: boolean;
  editFailures?: number;
  extraSessions?: number;
  quotaRemaining?: number | null;
}) {
  const [c] = useState(() =>
    resourceFixture({
      eraseFailures,
      usageDelayMs,
      usageFailures,
      longAccountDetails,
      editFailures,
      quotaRemaining,
    }),
  );
  const [memory] = useState(() => memoryFixture());
  return (
    <div
      style={{
        width:
          view === "accounts"
            ? "min(960px, 100vw)"
            : view === "memory"
              ? "min(850px, 100vw)"
              : "min(600px, 100vw)",
        boxSizing: "border-box",
        padding: "var(--space)",
        height: "95vh",
        overflow: view === "accounts" ? "hidden" : "auto",
        display: view === "accounts" ? "flex" : "block",
      }}
    >
      {view === "accounts" ? (
        <AccountPreview c={c} extraSessions={extraSessions} />
      ) : view === "memory" ? (
        <MemoryLibrary
          c={memory.c}
          project={new Id(memory.projects[0]!.id).toString()}
          sessions={memory.sessions}
        />
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
          "The connected resource components with isolated, typed in-memory RPC fixtures. For interactive account creation, login, retry, logout and session authentication recovery, see Accounts/Workflows. This general resource fixture does not simulate provider login.",
      },
    },
  },
  args: { view: "memory", quotaRemaining: 64 },
} satisfies Meta<typeof ResourcePreview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SharedMemory: Story = {};
export const Accounts: Story = { args: { view: "accounts" } };
export const AccountsLowQuota: Story = {
  args: { view: "accounts", quotaRemaining: 8 },
};
export const AccountsQuotaNotReported: Story = {
  args: { view: "accounts", quotaRemaining: null },
};
export const AccountsLongSessions: Story = {
  args: { view: "accounts", extraSessions: 20 },
};
export const AccountEraseFailure: Story = {
  args: { view: "accounts", eraseFailures: 1 },
};
export const AccountsLoading: Story = {
  args: { view: "accounts", usageDelayMs: 1500 },
};
export const AccountsLongDetails: Story = {
  args: { view: "accounts", longAccountDetails: true },
};
export const AccountEditFailure: Story = {
  args: { view: "accounts", editFailures: 1 },
};
export const Usage: Story = { args: { view: "usage" } };
export const UsageFailure: Story = {
  args: { view: "usage", usageFailures: 1 },
};
export const BackgroundTasks: Story = { args: { view: "background" } };
export const AuxiliaryAI: Story = { args: { view: "auxiliary" } };
