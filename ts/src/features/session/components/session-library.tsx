import { useState } from "react";
import { Button } from "@lesomnus/nox";
import { Connection } from "#src/shared/api/connection.ts";
import { useResourceInventory } from "#src/shared/api/resource-inventory.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { listAccounts } from "#src/features/accounts/account-data.ts";
import { SessionTable } from "#src/features/session/table/session-table.tsx";
import { SessionCreate } from "./session-create";
import type { Project } from "#gen/cxz/project_pb";
import { Icon } from "#src/shared/icons/icon.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import {
  ResourceLayout,
  ResourcePage,
  ResourceToolbar,
} from "#src/shared/components/resource-layout.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "#src/shared/components/resource-page.module.css";

export function SessionLibrary({
  c,
  projects,
}: {
  c: Connection;
  projects: Project[];
}) {
  const [creating, setCreating] = useState(false);
  const inventory = useResourceInventory(c);
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  return (
    <ResourcePage aria-label={t("Sessions")}>
      <ResourceToolbar
        actions={
          <>
            <Button
              variant="toolbar"
              aria-label={t("Add session")}
              onClick={() => setCreating(true)}
              disabled={!projects.length}
            >
              <Icon name="add" aria-hidden="true" />
            </Button>
            <Button
              variant="toolbar"
              aria-label={t("Refresh")}
              onClick={() => {
                c.inventory.refresh();
                void accounts.reload();
              }}
            >
              <Icon name="refresh" aria-hidden="true" />
            </Button>
          </>
        }
      />
      <PanelScroll
        contentClassName={styles.contentPadding}
        scrollbarLabel={t("Session list")}
        horizontalScrollbar
      >
        <ResourceLayout>
          {(inventory.errors.sessions || accounts.error) && (
            <ErrorNotice
              error={String(inventory.errors.sessions || accounts.error)}
            />
          )}
          <SessionTable
            c={c}
            sessions={inventory.sessions}
            accounts={accounts.data ?? []}
            projects={projects}
            loading={inventory.loading.sessions}
            variant="overview"
            showProject
            projectFirst
            showToolbar={false}
            scroll="page"
            addSession={() => setCreating(true)}
          />
        </ResourceLayout>
      </PanelScroll>
      {creating && (
        <SessionCreate
          c={c}
          projects={projects}
          centered
          close={() => setCreating(false)}
        />
      )}
    </ResourcePage>
  );
}
