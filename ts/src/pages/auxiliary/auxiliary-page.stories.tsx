import { useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import { resourceFixture } from "#src/storybook/resource-fixture.ts";
import { projectFixtureId } from "#src/storybook/project-id.ts";
import { AuxiliaryPage } from "./auxiliary-page";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { Provider } from "@lesomnus/payday/react";
import { createWorkspaceRouter } from "#src/app/router.tsx";
import { Workspace } from "#src/app/workspace-shell.tsx";
import { WorkspaceView } from "#src/app/workspace-view.tsx";
import type { Connection } from "#src/shared/api/connection.ts";
import { LoginTransportContext } from "#src/features/accounts/login-transport.ts";
import { PreviewLoginSocket } from "#src/storybook/accounts/account-workflow-fixture.ts";

function Preview({
  loading = false,
  noAccounts = false,
  saveFailure = false,
  longNames = false,
  pendingSave = false,
  quotaRemaining = 64,
  workspace = false,
  needsLogin = false,
  mixedProfiles = false,
}: {
  loading?: boolean;
  noAccounts?: boolean;
  saveFailure?: boolean;
  longNames?: boolean;
  pendingSave?: boolean;
  quotaRemaining?: number | null;
  workspace?: boolean;
  needsLogin?: boolean;
  mixedProfiles?: boolean;
}) {
  const [{ c, connect }] = useState(() => {
    let loginRequired = needsLogin;
    const c = resourceFixture({
      noAccounts,
      auxiliaryDelayMs: loading ? 60000 : 0,
      auxiliarySaveFailures: saveFailure ? 1 : 0,
      auxiliarySaveDelayMs: pendingSave ? 1000 : 0,
      longAccountDetails: longNames,
      quotaRemaining,
      auxiliaryNeedsLogin: () => loginRequired,
      auxiliaryMixedProfiles: mixedProfiles,
    });
    return {
      c,
      connect: () =>
        new PreviewLoginSocket("success", () => {
          loginRequired = false;
        }),
    };
  });
  const sessions = [
    create(SessionSchema, {
      runtimeId: "demo",
      agent: "codex",
      account: { id: projectFixtureId(10) },
    }),
    create(SessionSchema, {
      runtimeId: "demo-review",
      agent: "codex",
      account: { id: projectFixtureId(10) },
    }),
    create(SessionSchema, {
      runtimeId: "personal-demo",
      agent: "claude",
      account: { id: projectFixtureId(11) },
    }),
  ];
  useEffect(() => () => c.dispose(), [c]);
  return (
    <LoginTransportContext.Provider value={connect}>
      {workspace ? (
        <WorkspacePreview c={c} />
      ) : (
        <div style={{ display: "flex", height: "100dvh" }}>
          <AuxiliaryPage c={c} sessions={sessions} />
        </div>
      )}
    </LoginTransportContext.Provider>
  );
}

function WorkspacePreview({ c }: { c: Connection }) {
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <Workspace connection={c} logout={async () => {}}>
            {children}
          </Workspace>
        ),
        view: WorkspaceView,
      },
      createMemoryHistory({ initialEntries: ["/c/_/auxiliary"] }),
    ),
  );
  return (
    <Provider app={c}>
      <RouterProvider router={router} />
    </Provider>
  );
}
const meta = {
  title: "Pages/Auxiliary AI",
  component: Preview,
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "One header groups the shared account, model and effort Section selectors with helper authentication, login, save and revert actions. Summary, Suggestion and Title use wide clickable panels with descriptions and top-right switches, arranged in three columns on wider screens. Narrow layouts wrap the header and stack the panels. Revert restores the last saved configuration. Older different model/effort choices show Mixed until explicitly changed. Login is simulated; enter demo-code to finish.",
      },
    },
  },
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Loading: Story = { args: { loading: true } };
export const NoAccounts: Story = { args: { noAccounts: true } };
export const SaveFailure: Story = { args: { saveFailure: true } };
export const PendingSave: Story = { args: { pendingSave: true } };
export const LongNames: Story = { args: { longNames: true } };
export const UsageNotReported: Story = { args: { quotaRemaining: null } };
export const UsageDepleted: Story = { args: { quotaRemaining: 0 } };
export const UsageFull: Story = { args: { quotaRemaining: 100 } };
export const LoginRequired: Story = { args: { needsLogin: true } };
export const MixedConfiguration: Story = {
  args: { workspace: true, mixedProfiles: true },
};
export const InWorkspace: Story = {
  args: { workspace: true },
  parameters: {
    docs: {
      description: {
        story:
          "The real Workspace, router, resource sidebar and page composition. Auxiliary AI occupies the content columns beside the sidebar with no legacy resource navigation panel.",
      },
    },
  },
};
