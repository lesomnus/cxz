import { key } from "@lesomnus/payday/store";
import { CompactNumber, ValueMenu } from "@lesomnus/nox";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { sessionsForAccount } from "./account-data";
import { useAccountQuota } from "./use-account-quota";
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
              <AccountUsage
                c={c}
                account={account}
                sessions={sessionsForAccount(account, sessions)}
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

function AccountUsage({
  c,
  account,
  sessions,
}: {
  c: Connection;
  account: Account;
  sessions: Session[];
}) {
  const quota = useAccountQuota({
    c,
    agent: account.agent,
    sessions,
    identity: account.id.length ? key(account.id) : account.alias,
  });
  const remaining = quota.data?.remaining;
  return (
    <span className={styles.usage} aria-busy={quota.loading}>
      <span className={styles.remaining}>
        <small>Remaining</small>
        <span
          data-level={
            remaining === undefined
              ? "unknown"
              : remaining < 10
                ? "critical"
                : remaining <= 30
                  ? "warning"
                  : "normal"
          }
        >
          {remaining === undefined ? (
            <span className={styles.empty}>{quota.loading ? "--" : "—"}</span>
          ) : (
            <>
              <CompactNumber
                mode="integer"
                digits={remaining === 100 ? 3 : 2}
                value={Math.round(remaining)}
              />
              <span className={styles.unit}>%</span>
            </>
          )}
        </span>
      </span>
      <span className={styles.reset}>
        <small className={styles.resetLabel}>
          Resets
          <RelativeTime value={quota.data?.reset} />
        </small>
        <span
          className={quota.data?.reset === undefined ? styles.empty : undefined}
        >
          <DateTime value={quota.data?.reset} showOffset={false} />
        </span>
      </span>
    </span>
  );
}
