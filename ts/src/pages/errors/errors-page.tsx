import { Fragment, useId, useState, useSyncExternalStore } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  CompactNumber,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@lesomnus/nox";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import {
  ResourcePage,
  ResourceToolbar,
  ResourceLayout,
} from "#src/shared/components/resource-layout.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { ErrorStatusTabs } from "#src/shared/errors/error-status-tabs.tsx";
import {
  clientErrors,
  type ClientError,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";
import { resourceLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import pageStyles from "#src/shared/components/resource-page.module.css";
import resourceStyles from "#src/shared/components/resource-table.module.css";
import styles from "./errors-page.module.css";

const statusLabel = {
  active: "Active",
  resolved: "Resolved",
  dismissed: "Dismissed",
  stale: "Stale",
} as const;

function ErrorRow({
  entry,
  store,
}: {
  entry: ClientError;
  store: ClientErrors;
}) {
  const [expanded, setExpanded] = useState(false);
  const detailsId = useId();
  const status = t(statusLabel[entry.status]);
  return (
    <Fragment>
      <TableRow data-status={entry.status}>
        <TableCell className={styles.status}>
          <span>{status}</span>
        </TableCell>
        <TableCell className={styles.summary}>
          <div className={styles.message}>{entry.message}</div>
          {(entry.method || entry.target) && (
            <div className={styles.context}>
              {[entry.method, entry.target].filter(Boolean).join(" · ")}
            </div>
          )}
          <div className={styles.mobileMeta}>
            {status}
            <span>·</span>
            <RelativeTime value={entry.lastSeen} />
            <span>×</span>
            <CompactNumber value={entry.count} mode="integer" digits={3} />
          </div>
        </TableCell>
        <TableCell className={styles.count}>
          <CompactNumber value={entry.count} mode="integer" digits={3} />
        </TableCell>
        <TableCell className={styles.time}>
          <RelativeTime value={entry.lastSeen} />
        </TableCell>
        <TableCell className={styles.actions}>
          <div className={styles.rowActions}>
            <ToolbarIconButton
              icon="details"
              label={t("Details")}
              aria-expanded={expanded}
              aria-controls={detailsId}
              onClick={() => setExpanded((value) => !value)}
            />
            {entry.status === "active" && (
              <ToolbarIconButton
                icon="close"
                label={t("Dismiss")}
                onClick={() => store.dismiss(entry.id)}
              />
            )}
          </div>
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow className={styles.detailRow}>
          <TableCell colSpan={5}>
            <section
              id={detailsId}
              aria-label={t("Error details")}
              className={styles.detail}
            >
              <p className={styles.fullMessage}>{entry.message}</p>
              <dl className={styles.metadata}>
                <div>
                  <dt>{t("Status")}</dt>
                  <dd>{status}</dd>
                </div>
                <div>
                  <dt>{t("Count")}</dt>
                  <dd>
                    <CompactNumber
                      value={entry.count}
                      mode="integer"
                      digits={3}
                    />
                  </dd>
                </div>
                <div>
                  <dt>{t("First seen")}</dt>
                  <dd>
                    <DateTime value={entry.firstSeen} />
                  </dd>
                </div>
                <div>
                  <dt>{t("Last seen")}</dt>
                  <dd>
                    <DateTime value={entry.lastSeen} />
                  </dd>
                </div>
                {entry.closedAt && (
                  <div>
                    <dt>{status}</dt>
                    <dd>
                      <DateTime value={entry.closedAt} />
                    </dd>
                  </div>
                )}
                <div>
                  <dt>{t("Source")}</dt>
                  <dd>{entry.source}</dd>
                </div>
                {entry.code && (
                  <div>
                    <dt>{t("Code")}</dt>
                    <dd>{entry.code}</dd>
                  </div>
                )}
                {entry.method && (
                  <div>
                    <dt>{t("Method")}</dt>
                    <dd>{entry.method}</dd>
                  </div>
                )}
                {entry.target && (
                  <div>
                    <dt>{t("Target")}</dt>
                    <dd>{entry.target}</dd>
                  </div>
                )}
                {entry.connection && (
                  <div>
                    <dt>{t("Connection")}</dt>
                    <dd>{entry.connection}</dd>
                  </div>
                )}
                {entry.project && (
                  <div>
                    <dt>{t("Project")}</dt>
                    <dd>{entry.project}</dd>
                  </div>
                )}
                {entry.session && (
                  <div>
                    <dt>{t("Session")}</dt>
                    <dd>{entry.session}</dd>
                  </div>
                )}
                {entry.request && (
                  <div>
                    <dt>{t("Request ID")}</dt>
                    <dd>{entry.request}</dd>
                  </div>
                )}
              </dl>
            </section>
          </TableCell>
        </TableRow>
      )}
    </Fragment>
  );
}

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
          <Table
            className={`${styles.table} ${resourceStyles.layout}`}
            aria-label={t("Errors")}
          >
            <colgroup>
              <col className={styles.statusColumn} />
              <col />
              <col className={styles.countColumn} />
              <col className={styles.timeColumn} />
              <col className={styles.actionsColumn} />
            </colgroup>
            <TableHeader>
              <TableRow>
                <TableHead className={styles.status}>{t("Status")}</TableHead>
                <TableHead>{t("Message")}</TableHead>
                <TableHead className={styles.count}>{t("Count")}</TableHead>
                <TableHead className={styles.time}>{t("Last seen")}</TableHead>
                <TableHead aria-label={t("Actions")} />
              </TableRow>
            </TableHeader>
            <TableBody key={tab}>
              {shown.length ? (
                shown.map((entry) => (
                  <ErrorRow key={entry.id} entry={entry} store={store} />
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={5} className={styles.empty}>
                    {tab === "active"
                      ? t("No active errors")
                      : t("No error history")}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </ResourceLayout>
      </PanelScroll>
    </ResourcePage>
  );
}
