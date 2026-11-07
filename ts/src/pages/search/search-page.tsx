import type { Connection } from "#src/shared/api/connection.ts";
import type { Project } from "#gen/cxz/project_pb";
import { ConversationSearch } from "#src/features/search/conversation-search.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ResourcePage } from "#src/shared/components/resource-layout.tsx";
import styles from "./search-page.module.css";
export function SearchPage({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  return (
    <ResourcePage aria-label="Search workspace">
      <PanelScroll
        contentClassName={styles.content}
        scrollbarLabel="Search page scroll"
      >
        <ConversationSearch c={c} projects={projects} />
      </PanelScroll>
    </ResourcePage>
  );
}
