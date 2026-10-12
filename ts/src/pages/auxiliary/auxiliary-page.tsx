import type { Connection } from "#src/shared/api/connection.ts";
import { AuxiliarySettings } from "#src/features/auxiliary/auxiliary-settings.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
export function AuxiliaryPage({ c }: { c: Connection }) {
  return (
    <main className={styles.page}>
      <header className={styles.heading}>
        <h1>Auxiliary AI</h1>
      </header>
      <div className={styles.body}>
        <AuxiliarySettings c={c} />
      </div>
    </main>
  );
}
