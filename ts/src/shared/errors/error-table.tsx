import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
  type AnimationEventHandler,
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
import {
  ErrorDetailsDialog,
  errorStatusLabels as statusLabel,
} from "./error-details-dialog";
import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { RelativeTime } from "#src/shared/components/date-time.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import resourceStyles from "#src/shared/components/resource-table.module.css";
import styles from "./error-table.module.css";

const statusIcons = {
  active: "error-active",
  resolved: "error-resolved",
  dismissed: "hide",
  stale: "error-stale",
} as const satisfies Record<ClientError["status"], IconName>;
function useEntryEntrance(animateEnter: boolean) {
  const [animate, setAnimate] = useState(animateEnter);
  useEffect(() => {
    if (!animate) return;
    const timer = setTimeout(() => setAnimate(false), 300);
    return () => clearTimeout(timer);
  }, [animate]);
  const finish: AnimationEventHandler<HTMLTableRowElement> = (event) => {
    if (event.target === event.currentTarget) setAnimate(false);
  };
  return { animate, finish };
}

function ErrorRow({
  entry,
  store,
  animateEnter,
  details,
}: {
  entry: ClientError;
  store: ClientErrors;
  animateEnter: boolean;
  details: () => void;
}) {
  const view = presentError(entry);
  const status = t(statusLabel[entry.status]);
  const { animate, finish } = useEntryEntrance(animateEnter);
  return (
    <TableRow
      className={styles.pageRow}
      data-status={entry.status}
      data-animate-enter={animate || undefined}
      onAnimationEnd={finish}
    >
      <TableCell className={styles.state}>
        <span title={status}>
          <Icon
            name={statusIcons[entry.status]}
            role="img"
            aria-label={status}
          />
        </span>
      </TableCell>
      <TableCell className={styles.kind}>{view.kind}</TableCell>
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
          {view.kind}
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
            aria-haspopup="dialog"
            onClick={details}
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
  const { animate, finish } = useEntryEntrance(animateEnter);
  const view = presentError(entry);
  const status = t(statusLabel[entry.status]);
  const context = `${view.diagnostic}${view.target !== "—" ? ` · ${view.target}` : ""}`;
  return (
    <TableRow
      className={styles.dialogRow}
      data-animate-enter={animate || undefined}
      onAnimationEnd={finish}
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
  animateNewEntries = true,
}: {
  entries: readonly ClientError[];
  store: ClientErrors;
  tab: string;
  variant?: "page" | "dialog";
  animateNewEntries?: boolean;
}) {
  useLocale();
  const dialog = variant === "dialog";
  const [selected, setSelected] = useState<ClientError>();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const detail = selected
    ? (entries.find((entry) => entry.id === selected.id) ?? selected)
    : undefined;
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
        className={`${styles.table} ${resourceStyles.layout} ${dialog ? styles.dialogTable : resourceStyles.table}`}
        aria-label={t("Errors")}
      >
        {dialog ? (
          <colgroup>
            <col />
          </colgroup>
        ) : (
          <colgroup>
            <col className={styles.stateColumn} />
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
              <TableHead className={styles.state} aria-label={t("Status")} />
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
                  animateEnter={animateNewEntries && !knownIds.has(entry.id)}
                />
              ) : (
                <ErrorRow
                  key={entry.id}
                  entry={entry}
                  store={store}
                  animateEnter={animateNewEntries && !knownIds.has(entry.id)}
                  details={() => {
                    setSelected(entry);
                    setDetailsOpen(true);
                  }}
                />
              ),
            )
          ) : (
            <TableRow>
              <TableCell colSpan={dialog ? 1 : 8} className={styles.empty}>
                {tab === "all"
                  ? t("No errors in recent records")
                  : tab === "active"
                    ? t("No active errors")
                    : t("No error history")}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      {detail && (
        <ErrorDetailsDialog
          entry={detail}
          open={detailsOpen}
          close={() => setDetailsOpen(false)}
          closed={() => setSelected(undefined)}
        />
      )}
    </div>
  );
}
