import type { Connection } from "#src/shared/api/connection.ts";
import { AuxiliarySettings } from "#src/features/auxiliary/auxiliary-settings.tsx";
import type { Session } from "#gen/cxz/session_pb";
import {
  ResourceLayout,
  ResourcePage,
} from "#src/shared/components/resource-layout.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import styles from "#src/shared/components/resource-page.module.css";
export function AuxiliaryPage({
  c,
  sessions = [],
}: {
  c: Connection;
  sessions?: Session[];
}) {
  return (
    <ResourcePage aria-label="Auxiliary AI">
      <PanelScroll
        contentClassName={styles.contentPadding}
        scrollbarLabel="Auxiliary settings scroll"
      >
        <ResourceLayout>
          <AuxiliarySettings c={c} sessions={sessions} />
        </ResourceLayout>
      </PanelScroll>
    </ResourcePage>
  );
}
