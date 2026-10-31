import { useEffect, useState } from "react";
import { createMemoryHistory, RouterProvider } from "@tanstack/react-router";
import { Provider } from "@lesomnus/payday/react";
import { Button } from "@lesomnus/nox";
import { createWorkspaceRouter, useWorkspaceRoute } from "#src/app/router.tsx";
import { AccountsPage } from "#src/pages/accounts/accounts-page.tsx";
import { Conversation } from "#src/features/session/conversation/conversation.tsx";
import { SessionCreate } from "#src/features/session/components/session-create.tsx";
import { LoginTransportContext } from "#src/features/accounts/login-transport.ts";
import {
  accountWorkflowFixture,
  type AccountScenario,
  type LoginOutcome,
} from "./account-workflow-fixture";
import styles from "./account-workflows.module.css";

export function AccountWorkflows({
  scenario = "add",
  loginOutcome = "success",
}: {
  scenario?: AccountScenario;
  loginOutcome?: LoginOutcome;
}) {
  const [revision, setRevision] = useState(0);
  return (
    <div className={styles.frame}>
      <header className={styles.header}>
        <small>
          Local preview · use <code>demo-code</code> to sign in
        </small>
        <Button onClick={() => setRevision((n) => n + 1)}>Reset preview</Button>
      </header>
      <div className={styles.stage}>
        <WorkflowPreview
          key={`${scenario}/${loginOutcome}/${revision}`}
          scenario={scenario}
          loginOutcome={loginOutcome}
        />
      </div>
    </div>
  );
}

function WorkflowPreview({
  scenario,
  loginOutcome,
}: Required<Parameters<typeof AccountWorkflows>[0]>) {
  const [fixture] = useState(() =>
    accountWorkflowFixture(scenario, loginOutcome),
  );
  useEffect(() => () => fixture.dispose(), [fixture]);
  const [router] = useState(() => {
    if (scenario === "send")
      fixture.c.drafts.set(
        "login-preview",
        "Review the workspace and suggest the next step.",
      );
    return createWorkspaceRouter(
      {
        shell: ({ children }) => <>{children}</>,
        view: function WorkflowView() {
          const route = useWorkspaceRoute();
          const [creating, setCreating] = useState(false);
          if (route.session)
            return (
              <Conversation
                c={fixture.c}
                id="login-preview"
                projects={[fixture.project]}
              />
            );
          if (scenario === "create-session")
            return (
              <div className={styles.create}>
                <Button onClick={() => setCreating(true)}>Add session</Button>
                {creating && (
                  <SessionCreate
                    c={fixture.c}
                    projects={[fixture.project]}
                    centered
                    close={() => setCreating(false)}
                  />
                )}
              </div>
            );
          return <AccountsPage c={fixture.c} sessions={fixture.sessions} />;
        },
      },
      createMemoryHistory({
        initialEntries: [
          scenario === "send" || scenario === "expired"
            ? "/c/_/session/seal"
            : scenario === "create-session"
              ? "/c/_/session"
              : "/c/_/account",
        ],
      }),
    );
  });
  return (
    <Provider app={fixture.c}>
      <LoginTransportContext.Provider value={fixture.connect}>
        <RouterProvider router={router} />
      </LoginTransportContext.Provider>
    </Provider>
  );
}
