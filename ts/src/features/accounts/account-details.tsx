import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { useRef, useState } from "react";
import { ConnectError } from "@connectrpc/connect";
import {
  Button,
  EditableValue,
  Popover,
  SectionGroup,
  Section,
  Switch,
} from "@lesomnus/nox";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import { AccountAuthStatus_State as AuthState } from "#gen/cxz/account_svc_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { ResourceFields } from "#src/shared/components/resource-fields.tsx";
import { SessionTable } from "#src/features/session/table/session-table.tsx";
import { SessionActionIcon } from "#src/features/session/components/session-action-icon.tsx";
import { UsageReport } from "#src/features/activity/usage-report.tsx";
import {
  accountRef,
  accountStatus,
  editAccount,
  validateAccountField,
  type AccountField,
} from "./account-data";
import { LoginPanel } from "./login-panel";
import styles from "#src/shared/components/resource-surface.module.css";
import accountStyles from "./accounts.module.css";
export function AccountDetails({
  c,
  account,
  sessions,
  projects,
  sessionsLoading,
  erased,
  saved,
}: {
  c: Connection;
  account: Account;
  sessions: Session[];
  projects: Project[];
  sessionsLoading: boolean;
  erased: () => void;
  saved: (account: Account) => void;
}) {
  const editable = (field: AccountField, label: string) => (
    <div className={accountStyles.editable}>
      <EditableValue
        label={label}
        value={account[field]}
        monospace
        validate={(value) => validateAccountField(field, value)}
        save={async (value) => {
          const updated = await editAccount(c.accounts, account, field, value);
          c.store.apply("cxz.Account", [{ id: updated.id, value: updated }]);
          saved(updated);
        }}
      />
    </div>
  );
  const status = useRemote(
    (signal) => accountStatus(c, account.alias, signal),
    [c, account.alias],
  );
  const [login, setLogin] = useState(false),
    [logout, setLogout] = useState(false),
    [erase, setErase] = useState(false),
    [eraseError, setEraseError] = useState("");
  const didErase = useRef(false);
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
      <SectionGroup className={accountStyles.summaryColumn}>
        <Section
          aria-label="Account details"
          className={accountStyles.informationDetails}
        >
          <ResourceFields
            columns={2}
            rows={[
              ["Display name", editable("name", "Display name"), account.desc],
              ["Alias", editable("alias", "Alias")],
              [
                "Agent",
                <span className={accountStyles.agent}>
                  <AgentBrand agent={account.agent} />
                  {account.agent}
                </span>,
              ],
              [
                "Authentication backend",
                <span className={accountStyles.fieldValue}>
                  {account.authBackend || "—"}
                </span>,
              ],
              [
                "Created",
                <span className={accountStyles.fieldValue}>
                  <DateTime value={account.dateCreated} />
                </span>,
                undefined,
                <RelativeTime value={account.dateCreated} />,
              ],
            ]}
          />
        </Section>
        <Section
          aria-label="Sessions using this account"
          className={accountStyles.sessionsColumn}
        >
          <SessionTable
            c={c}
            sessions={sessions}
            projects={projects}
            accounts={[account]}
            loading={sessionsLoading}
            variant="compact"
            showAccount={false}
            showProject
            showToolbar={false}
            horizontalScrollbar
            fillHeight
          />
        </Section>
      </SectionGroup>
      <SectionGroup className={accountStyles.usageColumn}>
        <SectionGroup className={accountStyles.authenticationGroup}>
          <Section
            as="button"
            className={accountStyles.eraseAction}
            aria-label={`Erase account ${account.name || account.alias}`}
            onClick={() => {
              setEraseError("");
              setErase(true);
            }}
          >
            <SessionActionIcon action="erase" />
          </Section>
          <Section className={accountStyles.reservedControl}>
            <Switch
              label="Account option"
              checked={false}
              disabled
              onChange={() => {}}
            />
          </Section>
          <Section
            aria-label="Authentication"
            className={accountStyles.authenticationSection}
          >
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
                      content={
                        status.error || status.data?.message || statusText
                      }
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
                    onClick={() =>
                      signedIn ? setLogout(true) : setLogin(true)
                    }
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
          className={accountStyles.usageSection}
        />
      </SectionGroup>
      {erase && (
        <ConfirmationDialog
          title={`Erase account · ${account.name || account.alias}`}
          confirmLabel="Erase permanently"
          failureMessage={eraseError || undefined}
          close={() => {
            setErase(false);
            if (didErase.current) {
              didErase.current = false;
              erased();
            }
          }}
          execute={async () => {
            setEraseError("");
            try {
              await c.accounts.erase(
                account.id.length
                  ? { key: { case: "id", value: account.id } }
                  : accountRef(account.alias),
                { timeoutMs: 600000 },
              );
              didErase.current = true;
              return true;
            } catch (error) {
              setEraseError(ConnectError.from(error).rawMessage);
              return false;
            }
          }}
        >
          <p>
            Permanently erase <strong>{account.name || account.alias}</strong> (
            {account.alias}) and all sessions linked to this account.
          </p>
          <p>
            This deletes their conversations, memory, attachments and agent
            profiles, along with this account's saved login, authorizations and
            AI task configuration.
          </p>
          <p>
            Project workspaces and resources shared with other accounts are
            kept. This cannot be undone.
          </p>
        </ConfirmationDialog>
      )}
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
