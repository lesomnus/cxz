import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";
import { ConversationSearch } from "#src/features/search/conversation-search.tsx";
import styles from "#src/shared/components/resource-surface.module.css";
export function SearchPage({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  return (
    <main className={styles.page}>
      <header className={styles.heading}>
        <h1>Search</h1>
      </header>
      <div className={styles.body}>
        <ConversationSearch c={c} projects={projects} />
      </div>
    </main>
  );
}
