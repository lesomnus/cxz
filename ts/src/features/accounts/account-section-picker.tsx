import { key } from "@lesomnus/payday/store";
import { ValueMenu } from "@lesomnus/nox";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { sessionsForAccount } from "./account-data";
import { AccountQuotaSquare } from "./account-quota-square";
import styles from "./account-section-picker.module.css";

export function AccountSectionPicker({
  c,
  accounts,
  sessions,
  value,
  choose,
  disabled = false,
}: {
  c: Connection;
  accounts: Account[];
  sessions: Session[];
  value: string;
  choose: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <div className={styles.picker}>
      <ValueMenu
        label="Auxiliary account"
        variant="section"
        alignItemWithTrigger
        value={value}
        disabled={disabled || !accounts.length}
        options={[
          ...(!value
            ? [
                {
                  value: "",
                  label: accounts.length ? "Choose an account" : "No accounts",
                  description: "Account",
                  disabled: true,
                },
              ]
            : []),
          ...accounts.map((account) => ({
            value: account.alias,
            label: account.name || account.alias,
            description: account.alias,
            icon: <AgentBrand agent={account.agent} />,
            trailing: (
              <AccountQuotaSquare
                c={c}
                account={account.alias}
                agent={account.agent}
                identity={account.id.length ? key(account.id) : account.alias}
                sessions={sessionsForAccount(account, sessions)}
                inline
                className={styles.quota}
                gaugeClassName={styles.gauge}
              />
            ),
          })),
          ...(value && !accounts.some((account) => account.alias === value)
            ? [
                {
                  value,
                  label: "Unavailable account",
                  description: value,
                  muted: true,
                },
              ]
            : []),
        ]}
        choose={choose}
      />
    </div>
  );
}
