import {
  Fragment,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";
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
        <TableCell className={styles.operation}>
          <div className={styles.operationValue}>{view.diagnostic}</div>
          {view.operation !== view.diagnostic && (
            <div className={styles.operationLabel}>{view.operation}</div>
          )}
        </TableCell>
        <TableCell className={styles.target}>{view.target}</TableCell>
        <TableCell className={styles.summary}>
          <div className={styles.message}>{entry.message}</div>
          <div
            className={styles.context}
            title={`${view.operation} · ${view.diagnostic} · ${view.target}`}
          >
            {view.diagnostic}
            {view.target !== "—" && ` · ${view.target}`}
          </div>
          <div className={styles.mobileMeta}>
            {view.kind} · {status}
            <span>·</span>
            <RelativeTime value={entry.lastSeen} compact />
            <span>×</span>
            <CompactNumber value={entry.count} mode="integer" digits={3} />
          </div>
        </TableCell>
        <TableCell className={styles.count}>
          <CompactNumber value={entry.count} mode="integer" digits={3} />
        </TableCell>
        <TableCell className={styles.time}>
          <RelativeTime value={entry.lastSeen} compact />
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

function DialogErrorRow({
  entry,
  store,
  animateEnter,
}: {
  entry: ClientError;
  store: ClientErrors;
  animateEnter: boolean;
}) {
  // A new row keeps its own entrance even if another report arrives mid-animation.
  const [animate, setAnimate] = useState(animateEnter);
  useEffect(() => {
    if (!animate) return;
    // Also clear reduced/canceled entrances so reordering cannot replay them later.
    const timer = setTimeout(() => setAnimate(false), 300);
    return () => clearTimeout(timer);
  }, [animate]);
  const view = presentError(entry);
  const status = t(statusLabel[entry.status]);
  const context = `${view.diagnostic}${view.target !== "—" ? ` · ${view.target}` : ""}`;
  return (
    <TableRow
      className={styles.dialogRow}
      data-animate-enter={animate || undefined}
      onAnimationEnd={(event) => {
        if (event.target === event.currentTarget) setAnimate(false);
      }}
      data-status={entry.status}
      title={`${status} · ${view.target}`}
    >
      <TableCell className={styles.summary}>
        <span className={styles.srOnly}>{status}</span>
        <div className={styles.dialogMessage} title={entry.message}>
          {entry.message}
        </div>
        <div className={styles.dialogMeta}>
          <span
            className={styles.dialogOperation}
            title={`${view.operation} · ${context}`}
          >
            {context}
          </span>
          <span className={styles.dialogTime}>
            <RelativeTime value={entry.lastSeen} compact />
          </span>
          <span
            className={styles.dialogCount}
            title={`${entry.count} ${t("occurrences")}`}
          >
            <CompactNumber value={entry.count} mode="integer" digits={3} />
          </span>
        </div>
        {entry.status === "active" && (
          <div className={styles.dialogDismiss}>
            <ToolbarIconButton
              className={styles.dismissButton}
              icon="close"
              label={t("Dismiss")}
              onClick={() => store.dismiss(entry.id)}
            />
          </div>
        )}
      </TableCell>
    </TableRow>
  );
}

export function ErrorTable({
  entries,
  store,
  tab,
  variant = "page",
}: {
  entries: readonly ClientError[];
  store: ClientErrors;
  tab: string;
  variant?: "page" | "dialog";
}) {
  useLocale();
  const dialog = variant === "dialog";
  // Baseline includes both tabs. Opening the dialog or changing tabs is not a new error.
  const knownIds = useMemo(
    () => new Set(store.snapshot().map((entry) => entry.id)),
    [store],
  );
  useLayoutEffect(() => {
    knownIds.clear();
    for (const entry of store.snapshot()) knownIds.add(entry.id);
  }, [store, entries, knownIds]);
  return (
    <div className={styles.frame}>
      <Table
        className={`${styles.table} ${resourceStyles.layout} ${dialog ? styles.dialogTable : ""}`}
        aria-label={t("Errors")}
      >
        {dialog ? (
          <colgroup>
            <col />
          </colgroup>
        ) : (
          <colgroup>
            <col className={styles.kindColumn} />
            <col className={styles.operationColumn} />
            <col className={styles.targetColumn} />
            <col />
            <col className={styles.countColumn} />
            <col className={styles.timeColumn} />
            <col className={styles.actionsColumn} />
          </colgroup>
        )}
        {!dialog && (
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
            entries.map((entry) =>
              dialog ? (
                <DialogErrorRow
                  key={entry.id}
                  entry={entry}
                  store={store}
                  animateEnter={!knownIds.has(entry.id)}
                />
              ) : (
                <ErrorRow key={entry.id} entry={entry} store={store} />
              ),
            )
          ) : (
            <TableRow>
              <TableCell colSpan={dialog ? 1 : 7} className={styles.empty}>
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
