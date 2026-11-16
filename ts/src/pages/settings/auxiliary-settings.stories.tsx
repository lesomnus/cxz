import { useContext, useEffect, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "#gen/cxz/session_pb";
import { resourceFixture } from "#src/storybook/resource-fixture.ts";
import { projectFixtureId } from "#src/storybook/project-id.ts";
import { SettingsPage } from "./settings-page";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { Provider } from "@lesomnus/payday/react";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { WorkspaceContext } from "#src/app/workspace-context.tsx";
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
  needsLogin = false,
  mixedProfiles = false,
}: {
  loading?: boolean;
  noAccounts?: boolean;
  saveFailure?: boolean;
  longNames?: boolean;
  pendingSave?: boolean;
  quotaRemaining?: number | null;
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
      <WorkspacePreview c={c} sessions={sessions} />
    </LoginTransportContext.Provider>
  );
}

function WorkspacePreview({
  c,
  sessions,
}: {
  c: Connection;
  sessions: import("#gen/cxz/session_pb").Session[];
}) {
  function PreviewView() {
    const workspace = useContext(WorkspaceContext)!;
    const route = useWorkspaceRoute();
    return route.resource === "settings" ? (
      <SettingsPage
        c={c}
        sessions={sessions}
        topic={route.settingsTopic}
        fileOpen={workspace.settingsFileOpen}
        setFileOpen={workspace.setSettingsFileOpen}
      />
    ) : (
      <WorkspaceView />
    );
  }
  const [router] = useState(() =>
    createWorkspaceRouter(
      {
        shell: ({ children }) => (
          <Workspace connection={c} logout={async () => {}}>
            {children}
          </Workspace>
        ),
        view: PreviewView,
      },
      createMemoryHistory({ initialEntries: ["/setting/auxiliary"] }),
    ),
  );
  return (
    <Provider app={c}>
      <RouterProvider router={router} />
    </Provider>
  );
}
const meta = {
  title: "Pages/Settings/Auxiliary AI",
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
  args: { mixedProfiles: true },
};
export const InWorkspace: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "The real Settings page and workspace. Auxiliary AI is a settings topic with the existing server-backed account, model, effort and login workflows. Profile owns account navigation and its hover usage preview; Accounts and Auxiliary AI no longer have dedicated sidebar buttons.",
      },
    },
  },
};
