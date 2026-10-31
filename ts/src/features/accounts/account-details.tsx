import { sessionLocation } from "#src/shared/navigation/resource-route.ts";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { useState } from "react";
import { Button, Popover, SectionGroup, Section } from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import { AccountAuthStatus_State as AuthState } from "#gen/cxz/account_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import { SessionIdentity } from "#src/features/session/components/session-identity.tsx";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import { accountRef, accountStatus } from "./account-data";
import { LoginPanel } from "./login-panel";
import styles from "#src/shared/components/resource-surface.module.css";
import accountStyles from "./accounts.module.css";
export function AccountDetails({
  c,
  account,
  sessions,
  sessionsLoading,
}: {
  c: Connection;
  account: Account;
  sessions: Session[];
  sessionsLoading: boolean;
}) {
  const status = useRemote(
    (signal) => accountStatus(c, account.alias, signal),
    [c, account.alias],
  );
  const [login, setLogin] = useState(false),
    [logout, setLogout] = useState(false);
  const labels = {
    [AuthState.READY]: "Signed in",
    [AuthState.LOGIN_REQUIRED]: "Login required",
    [AuthState.SESSION_SCOPED]: "Session-scoped login",
    [AuthState.BUSY]: "Authentication busy",
    [AuthState.UNSPECIFIED]: "Unknown",
  };
  const sessionScoped = status.data
    ? status.data.scope === "session" ||
      status.data.state === AuthState.SESSION_SCOPED
    : account.agent === "claude";
  const signedIn = status.data?.state === AuthState.READY;
  const statusText = status.loading
    ? "Checking login…"
    : status.data
      ? labels[status.data.state ?? AuthState.UNSPECIFIED]
      : "Status unavailable";
  return (
    <div className={accountStyles.details}>
      <SectionGroup className={accountStyles.informationColumn}>
        <Section
          aria-label="Account details"
          className={accountStyles.informationDetails}
        >
          <ResourceFields
            columns={1}
            rows={[
              ["Display name", account.name || account.alias],
              ["Alias", account.alias],
              [
                "Agent",
                <span className={accountStyles.agent}>
                  <AgentBrand agent={account.agent} />
                  {account.agent}
                </span>,
              ],
              ["Authentication backend", account.authBackend],
              ["Created", <DateTime value={account.dateCreated} />],
            ]}
          />
          {account.desc && <p>{account.desc}</p>}
        </Section>
        <Section aria-label="Authentication">
          <div className={accountStyles.authenticationRow}>
            {sessionScoped ? (
              <small
                className={`${styles.muted} ${accountStyles.authenticationDescription}`}
              >
                {status.data?.message ||
                  "Login belongs to each session. Manage it from that session's details."}
              </small>
            ) : (
              <>
                <span
                  className={accountStyles.authenticationStatus}
                  role={status.error ? "alert" : undefined}
                  aria-description={status.error || undefined}
                >
                  <Popover
                    content={status.error || status.data?.message || statusText}
                  >
                    {statusText}
                  </Popover>
                </span>
                <Button
                  className={accountStyles.authenticationAction}
                  disabled={
                    status.loading ||
                    login ||
                    logout ||
                    status.data?.scope !== "account" ||
                    !(signedIn ? status.data.canLogout : status.data.canLogin)
                  }
                  onClick={() => (signedIn ? setLogout(true) : setLogin(true))}
                >
                  {signedIn ? "Log out" : "Log in"}
                </Button>
              </>
            )}
          </div>
          {login && (
            <LoginPanel
              modal
              c={c}
              target={{ account: account.alias }}
              complete={() => {
                setLogin(false);
                status.reload();
              }}
              cancel={() => {
                setLogin(false);
                status.reload();
              }}
            />
          )}
        </Section>
      </SectionGroup>
      <UsageReport
        c={c}
        account={account.alias}
        className={accountStyles.usageColumn}
      />
      <Section
        aria-label="Sessions using this account"
        className={accountStyles.sessionsColumn}
      >
        <strong>Sessions · {sessions.length}</strong>
        {sessionsLoading && <p className={styles.muted}>Loading sessions…</p>}
        {!sessionsLoading && !sessions.length && (
          <p className={styles.muted}>No sessions use this account.</p>
        )}
        {sessions.map((session) => (
          <RouteLink
            key={session.runtimeId}
            {...sessionLocation(session)}
            className={accountStyles.session}
          >
            <SessionIdentity session={session} />
            <span className={styles.muted}>
              {session.status?.state || "unknown"}
            </span>
          </RouteLink>
        ))}
      </Section>
      {logout && (
        <ConfirmationDialog
          title={`Log out · ${account.name || account.alias}`}
          confirmLabel="Log out"
          close={() => setLogout(false)}
          execute={async () => {
            await c.accounts.logout(accountRef(account.alias), {
              timeoutMs: 10000,
            });
            status.reload();
            return true;
          }}
        >
          <p>
            Remove this account's saved login and revoke future token supply.
            Running sessions may keep tokens already received.
          </p>
        </ConfirmationDialog>
      )}
    </div>
  );
}
