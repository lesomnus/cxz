import type { Connection } from "#src/shared/api/connection.ts";
import { AccountList } from "#src/features/accounts/account-list.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
export function AccountsPage({ c }: { c: Connection }) {
  return (
    <main className={styles.page}>
      <header className={styles.heading}>
        <h1>Accounts</h1>
      </header>
      <div className={styles.body}>
        <AccountList c={c} />
      </div>
    </main>
  );
}
