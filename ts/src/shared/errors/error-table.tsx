import { Fragment, useId, useState } from "react";
import {
  CompactNumber,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@lesomnus/nox";
import type { ClientError, ClientErrors } from "./client-errors";
import { presentError } from "./error-presentation";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import resourceStyles from "#src/shared/components/resource-table.module.css";
import styles from "./error-table.module.css";

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
  const view = presentError(entry);
  return (
    <Fragment>
      <TableRow data-status={entry.status}>
        <TableCell className={styles.kind}>
          <span>{view.kind}</span>
          <span className={styles.status}>{status}</span>
        </TableCell>
        <TableCell className={styles.operation}>{view.operation}</TableCell>
        <TableCell className={styles.target}>{view.target}</TableCell>
        <TableCell className={styles.summary}>
          <div className={styles.message}>{entry.message}</div>
          <div className={styles.context}>
            {view.operation} · {view.target}
          </div>
          <div className={styles.mobileMeta}>
            {view.kind} · {status}
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
          <TableCell colSpan={7}>
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
                  <dt>{t("Kind")}</dt>
                  <dd>{view.kind}</dd>
                </div>
                <div>
                  <dt>{t("Operation")}</dt>
                  <dd>{view.operation}</dd>
                </div>
                {view.details.map(([label, value]) => (
                  <div key={label}>
                    <dt>{t(label)}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </section>
          </TableCell>
        </TableRow>
      )}
    </Fragment>
  );
}

export function ErrorTable({
  entries,
  store,
  tab,
  showHeader = true,
}: {
  entries: readonly ClientError[];
  store: ClientErrors;
  tab: string;
  showHeader?: boolean;
}) {
  useLocale();
  return (
    <div className={styles.frame}>
      <Table
        className={`${styles.table} ${resourceStyles.layout}`}
        aria-label={t("Errors")}
      >
        <colgroup>
          <col className={styles.kindColumn} />
          <col className={styles.operationColumn} />
          <col className={styles.targetColumn} />
          <col />
          <col className={styles.countColumn} />
          <col className={styles.timeColumn} />
          <col className={styles.actionsColumn} />
        </colgroup>
        {showHeader && (
          <TableHeader>
            <TableRow>
              <TableHead className={styles.kind}>{t("Kind")}</TableHead>
              <TableHead className={styles.operation}>
                {t("Operation")}
              </TableHead>
              <TableHead className={styles.target}>{t("Target")}</TableHead>
              <TableHead>{t("Message")}</TableHead>
              <TableHead className={styles.count}>{t("Count")}</TableHead>
              <TableHead className={styles.time}>{t("Last seen")}</TableHead>
              <TableHead aria-label={t("Actions")} />
            </TableRow>
          </TableHeader>
        )}
        <TableBody key={tab}>
          {entries.length ? (
            entries.map((entry) => (
              <ErrorRow key={entry.id} entry={entry} store={store} />
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={7} className={styles.empty}>
                {tab === "active"
                  ? t("No active errors")
                  : t("No error history")}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
