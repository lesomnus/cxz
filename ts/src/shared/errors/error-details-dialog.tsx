import { useRef } from "react";
import {
  CompactNumber,
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from "@lesomnus/nox";
import type { ClientError } from "./client-errors";
import { presentError } from "./error-presentation";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import { DateTime } from "#src/shared/components/date-time.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./error-table.module.css";

export const errorStatusLabels = {
  active: "Active",
  resolved: "Resolved",
  dismissed: "Dismissed",
  stale: "Stale",
} as const;
export function ErrorDetailsDialog({
  entry,
  open,
  close,
  closed,
}: {
  entry: ClientError;
  open: boolean;
  close: () => void;
  closed: () => void;
}) {
  const button = useRef<HTMLButtonElement>(null);
  const view = presentError(entry);
  const status = t(errorStatusLabels[entry.status]);
  return (
    <Dialog
      open={open}
      onClose={close}
      onClosed={closed}
      initialFocus={button}
      className={styles.detailsDialog}
    >
      <DialogHeader
        className={styles.detailsHeader}
        style={{ padding: "var(--ui-control-inset)", minHeight: 24 }}
      >
        <DialogTitle className={styles.detailsTitle}>
          {t("Error details")}
        </DialogTitle>
        <ToolbarIconButton
          ref={button}
          icon="close"
          label={t("Close details")}
          className={styles.detailsClose}
          onClick={close}
        />
      </DialogHeader>
      <DialogBody className={styles.detailsBody}>
        <p className={styles.fullMessage}>{entry.message}</p>
        <dl className={styles.metadata}>
          <div>
            <dt>{t("Status")}</dt>
            <dd>{status}</dd>
          </div>
          <div>
            <dt>{t("Notifications")}</dt>
            <dd>
              {entry.notify === false
                ? t("Excluded from alerts")
                : t("Included in alerts")}
            </dd>
          </div>
          <div>
            <dt>{t("Count")}</dt>
            <dd>
              <CompactNumber value={entry.count} mode="integer" digits={3} />
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
          {entry.closedAt !== undefined && (
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
      </DialogBody>
    </Dialog>
  );
}
