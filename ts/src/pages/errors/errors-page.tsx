import { useSyncExternalStore } from "react";
import {
  ResourcePage,
  ResourceToolbar,
  ResourceLayout,
} from "#src/shared/components/resource-layout.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
import styles from "./errors-page.module.css";
import { ErrorTable } from "#src/shared/errors/error-table.tsx";

export function ErrorsPage({
  store = clientErrors(),
}: {
  store?: ClientErrors;
}) {
  useLocale();
  const entries = useSyncExternalStore(
    store.subscribe,
    store.snapshot,
    store.snapshot,
  );
  const history = entries.filter((entry) => entry.status !== "active");
  const shown = [...entries].sort(
    (a, b) =>
      Number(b.status === "active") - Number(a.status === "active") ||
      b.updated - a.updated,
  );
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
      />
      <PanelScroll
        className={styles.scroll}
        contentClassName={pageStyles.contentPadding}
        scrollbarLabel={t("Scroll errors")}
        aria-label={t("Error list")}
      >
        <ResourceLayout>
          <ErrorTable entries={shown} store={store} tab="all" />
        </ResourceLayout>
      </PanelScroll>
    </ResourcePage>
  );
}
