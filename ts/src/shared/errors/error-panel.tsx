import { useSyncExternalStore } from "react";
import { Card, ScrollArea } from "@lesomnus/nox";
import { ErrorStatusTabs } from "./error-status-tabs";
import type { ClientErrors } from "#src/shared/errors/client-errors.ts";
import { ErrorTable } from "./error-table";
import { RouteLink } from "#src/shared/navigation/route-link.tsx";
import { resourceLocation } from "#src/shared/navigation/resource-route.ts";
import { Icon } from "#src/shared/icons/icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./error-panel.module.css";

export function ErrorPanel({
  store,
  tab,
  onTabChange,
  close,
}: {
  store: ClientErrors;
  tab: string;
  onTabChange: (tab: string) => void;
  close?: () => void;
}) {
  useLocale();
  const entries = useSyncExternalStore(
    store.subscribe,
    store.snapshot,
    store.snapshot,
  );
  const active = entries.filter((entry) => entry.status === "active");
  const history = entries.filter((entry) => entry.status !== "active");
  const shown = tab === "history" ? history : active;
  return (
    <Card
      revealFrom="left"
      className={styles.panel}
      title={t("Errors")}
      close={close}
      closeLabel={t("Close errors")}
      bodyPadding={0}
      headerCenter={
        <div className={styles.headerControls}>
          <ErrorStatusTabs
            active={active.length}
            history={history.length}
            tab={tab}
            onChange={onTabChange}
          />
          <RouteLink
            {...resourceLocation("errors")}
            search={{ tab: tab === "history" ? "history" : undefined }}
            variant="toolbar"
            className={styles.pageLink}
            aria-label={t("Open errors page")}
            title={t("Open errors page")}
            onClick={(event) => {
              if (
                !event.ctrlKey &&
                !event.metaKey &&
                !event.shiftKey &&
                !event.altKey
              )
                close?.();
            }}
          >
            <Icon name="open-external" />
          </RouteLink>
        </div>
      }
    >
      <ScrollArea
        className={styles.list}
        scrollbarOffset={8}
        aria-label={t("Error list")}
        scrollbarLabel={t("Scroll errors")}
      >
        <ErrorTable entries={shown} store={store} tab={tab} variant="dialog" />
      </ScrollArea>
    </Card>
  );
}
