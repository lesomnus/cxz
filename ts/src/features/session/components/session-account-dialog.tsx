import { useState } from "react";
import { FormField, ValueMenu } from "@lesomnus/nox";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { resumableStates } from "#src/features/session/model/session-actions.ts";

/** Choose another account of the session's agent and rebind the session to it. */
export function SessionAccountDialog({
  c,
  session,
  switchAccount,
  close,
}: {
  c: Pick<Connection, "accounts">;
  session: Session;
  switchAccount: (alias: string) => Promise<boolean>;
  close: () => void;
}) {
  useLocale();
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const current = session.account?.alias ?? "";
  const choices = (accounts.data ?? []).filter(
    (a) => a.agent === session.agent && a.alias !== current,
  );
  const [alias, setAlias] = useState("");
  const running = !resumableStates.has(session.status?.state ?? "");
  return (
    <ConfirmationDialog
      title={t("Change account")}
      confirmLabel={running ? t("Stop and change") : t("Change")}
      close={close}
      failureMessage={t(
        "Could not change the account. Check session status before retrying.",
      )}
      confirmDisabled={!alias}
      execute={() => switchAccount(alias)}
    >
      <p>
        {t(
          "The conversation, its history and its settings stay with this session. The new account may ask you to log in before the session runs again.",
        )}
      </p>
      {running && (
        <p className="muted">
          {t("The session is stopped first. Active work stops.")}
        </p>
      )}
      <FormField label={t("Account")}>
        <ValueMenu
          label={t("Account")}
          reserveDescriptionSpace
          alignItemWithTrigger
          value={alias}
          options={[
            { value: "", label: t("Choose an account") },
            ...choices.map((a) => ({
              value: a.alias,
              label: a.name || a.alias,
              description: a.alias,
              icon: <AgentBrand agent={a.agent} />,
            })),
          ]}
          choose={setAlias}
        />
      </FormField>
      {accounts.data && !choices.length && (
        <p className="muted">
          {t("No other account is registered for this agent.")}
        </p>
      )}
      {accounts.error && <ErrorNotice error={accounts.error} />}
    </ConfirmationDialog>
  );
}
