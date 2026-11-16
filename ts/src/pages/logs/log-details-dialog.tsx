import { useRef } from "react";
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogBody,
  ScrollArea,
} from "@lesomnus/nox";
import { ToolbarIconButton } from "#src/shared/components/toolbar-icon-button.tsx";
import type { LogEntry } from "#src/shared/logs/log-entry.ts";
import {
  ResourceFields,
  type ResourceFieldRow,
} from "#src/shared/components/resource-fields.tsx";
import { DateTime, RelativeTime } from "#src/shared/components/date-time.tsx";
import { LogLevelIcon } from "./log-level-icon";
import { LogCategoryIcon } from "./log-category-icon";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./log-details-dialog.module.css";

export function LogDetailsDialog({
  entry,
  open,
  close,
  closed,
}: {
  entry: LogEntry;
  open: boolean;
  close: () => void;
  closed: () => void;
}) {
  const button = useRef<HTMLButtonElement>(null);
  const context: ResourceFieldRow[] = [
    {
      label: t("Connection URL"),
      value: entry.connection,
      fullWidth: true,
      className: styles.text,
    },
    { label: t("Project UUID"), value: entry.project, identifier: true },
    { label: t("Session UUID"), value: entry.session, identifier: true },
    {
      label: t("Method"),
      value: entry.method,
      fullWidth: true,
      className: styles.text,
    },
    { label: t("Request ID"), value: entry.request, identifier: true },
  ].filter((field) => field.value !== undefined && field.value !== "");
  const details = Object.entries(entry.details ?? {}).filter(
    ([, value]) => value !== undefined,
  );
  return (
    <Dialog
      open={open}
      onClose={close}
      onClosed={closed}
      initialFocus={button}
      className={styles.dialog}
    >
      <DialogHeader>
        <DialogTitle>{t("Log details")}</DialogTitle>
        <ToolbarIconButton
          ref={button}
          className={styles.close}
          icon="close"
          label={t("Close details")}
          onClick={close}
        />
      </DialogHeader>
      <DialogBody className={styles.body}>
        <ScrollArea
          className={styles.scroll}
          contentClassName={styles.content}
          scrollbarLabel={t("Log details")}
        >
          <ResourceFields
            rows={[
              {
                label: t("Message"),
                value: entry.message,
                fullWidth: true,
                className: styles.text,
              },
              {
                label: t("Time"),
                value: <DateTime value={entry.time} />,
                labelDetail: <RelativeTime value={entry.time} />,
                fullWidth: true,
              },
              {
                label: t("Level"),
                value: (
                  <span className={styles.kind}>
                    <LogLevelIcon level={entry.level} />
                    {entry.level}
                  </span>
                ),
              },
              {
                label: t("Category"),
                value: (
                  <span className={styles.kind}>
                    <LogCategoryIcon category={entry.category} />
                    {entry.category === "rpc" ? "RPC" : t("Client")}
                  </span>
                ),
              },
              ...context,
              { label: t("Log ID"), value: entry.id, identifier: true },
              { label: t("Tab ID"), value: entry.tab, identifier: true },
              {
                label: t("Client version"),
                value: entry.version,
                fullWidth: true,
                className: styles.text,
              },
            ]}
          />
          {!!details.length && (
            <section aria-label={t("Details")}>
              <h3 className={styles.heading}>{t("Details")}</h3>
              <ResourceFields
                rows={details.map(([label, value]) => ({
                  label,
                  value: value === "" ? '""' : String(value),
                  fullWidth: typeof value === "string",
                  className: styles.text,
                }))}
              />
            </section>
          )}
        </ScrollArea>
      </DialogBody>
    </Dialog>
  );
}
