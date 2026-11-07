import { useSyncExternalStore } from "react";
import {
  Button,
  Card,
  CardActions,
  ScrollArea,
  SegmentedControl,
  CompactNumber,
} from "@lesomnus/nox";
import type { ClientErrors } from "#src/shared/errors/client-errors.ts";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
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
  mode = "dialog",
  close,
}: {
  store: ClientErrors;
  tab: string;
  onTabChange: (tab: string) => void;
  mode?: "dialog" | "page";
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
      revealFrom={mode === "dialog" ? "left" : undefined}
      fill={mode === "page"}
      className={mode === "page" ? styles.pagePanel : undefined}
      title={t("Errors")}
      close={close}
      closeLabel={t("Close errors")}
      bodyPadding={0}
      headerCenter={
        <div className={styles.headerControls}>
          <SegmentedControl
            label={t("Error status")}
            size="compact"
            value={tab}
            onChange={onTabChange}
            options={[
              {
                value: "active",
                label: `${t("Active")} ${active.length}`,
                display: (
                  <>
                    {t("Active")}
                    <CompactNumber
                      value={active.length}
                      mode="integer"
                      digits={3}
                    />
                  </>
                ),
              },
              {
                value: "history",
                label: `${t("History")} ${history.length}`,
                display: (
                  <>
                    {t("History")}
                    <CompactNumber
                      value={history.length}
                      mode="integer"
                      digits={3}
                    />
                  </>
                ),
              },
            ]}
          />
          {mode === "dialog" && (
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
          )}
        </div>
      }
      footer={
        <CardActions>
          <Button
            disabled={!history.length}
            onClick={() => store.clearHistory()}
          >
            {t("Clear history")}
          </Button>
        </CardActions>
      }
    >
      <ScrollArea
        className={styles.list}
        aria-label={t("Error list")}
        scrollbarLabel={t("Scroll errors")}
      >
        {!shown.length ? (
          <p className={styles.empty}>
            {tab === "active" ? t("No active errors") : t("No error history")}
          </p>
        ) : (
          <ul>
            {shown.map((entry) => (
              <li key={entry.id} data-status={entry.status}>
                <div className={styles.row}>
                  <span className={styles.state}>
                    {t(
                      entry.status === "active"
                        ? "Active"
                        : entry.status === "resolved"
                          ? "Resolved"
                          : entry.status === "stale"
                            ? "Stale"
                            : "Dismissed",
                    )}
                  </span>
                  <span className={styles.time}>
                    <RelativeTime value={entry.lastSeen} />
                  </span>
                  {entry.status === "active" && (
                    <Button
                      className={styles.dismiss}
                      onClick={() => store.dismiss(entry.id)}
                    >
                      {t("Dismiss")}
                    </Button>
                  )}
                </div>
                <p className={styles.message}>{entry.message}</p>
                {entry.method && (
                  <code className={styles.method}>{entry.method}</code>
                )}
                {entry.target && (
                  <code className={styles.method}>{entry.target}</code>
                )}
                <div className={styles.details}>
                  {entry.code && <span>{entry.code}</span>}
                  {entry.count > 1 && (
                    <span>
                      <CompactNumber
                        value={entry.count}
                        mode="integer"
                        digits={3}
                      />{" "}
                      {t("occurrences")}
                    </span>
                  )}
                  {entry.connection && <span>{entry.connection}</span>}
                </div>
                <div className={styles.details}>
                  <span>
                    {t("First seen")}: <DateTime value={entry.firstSeen} />
                  </span>
                  {entry.closedAt && (
                    <span>
                      {t(
                        entry.status === "resolved"
                          ? "Resolved"
                          : entry.status === "stale"
                            ? "Stale"
                            : "Dismissed",
                      )}
                      : <DateTime value={entry.closedAt} />
                    </span>
                  )}
                </div>
                {(entry.session || entry.project) && (
                  <code className={styles.method}>
                    {entry.session
                      ? `Session ${entry.session}`
                      : `Project ${entry.project}`}
                  </code>
                )}
              </li>
            ))}
          </ul>
        )}
      </ScrollArea>
    </Card>
  );
}
