import { useSyncExternalStore } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import {
  ResourcePage,
  ResourceToolbar,
  ResourceLayout,
} from "#src/shared/components/resource-layout.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { ErrorStatusTabs } from "#src/shared/errors/error-status-tabs.tsx";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";
import { resourceLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
import { ErrorTable } from "#src/shared/errors/error-table.tsx";

export function ErrorsPage({
  store = clientErrors(),
}: {
  store?: ClientErrors;
}) {
  useLocale();
  const { errorTab } = useWorkspaceRoute();
  const navigate = useNavigate();
  const entries = useSyncExternalStore(
    store.subscribe,
    store.snapshot,
    store.snapshot,
  );
  const active = entries.filter((entry) => entry.status === "active");
  const history = entries.filter((entry) => entry.status !== "active");
  const tab = errorTab ?? "active";
  const shown = tab === "active" ? active : history;
  return (
    <ResourcePage aria-label={t("Errors")}>
      <ResourceToolbar
        aria-label={t("Errors")}
        actions={
          <ToolbarIconButton
            icon="erase"
            label={t("Clear history")}
            disabled={!history.length}
            onClick={() => store.clearHistory()}
          />
        }
      >
        <ErrorStatusTabs
          active={active.length}
          history={history.length}
          tab={tab}
          onChange={(tab) =>
            void navigate({
              ...resourceLocation("errors"),
              search: { tab: tab === "history" ? "history" : undefined },
              replace: true,
            })
          }
        />
      </ResourceToolbar>
      <PanelScroll
        contentClassName={pageStyles.contentPadding}
        scrollbarLabel={t("Scroll errors")}
        aria-label={t("Error list")}
      >
        <ResourceLayout>
          <ErrorTable entries={shown} store={store} tab={tab} />
        </ResourceLayout>
      </PanelScroll>
    </ResourcePage>
  );
}
