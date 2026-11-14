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
        <ScrollArea className={styles.scroll} scrollbarLabel={t("Log details")}>
          <pre className={styles.record}>{JSON.stringify(entry, null, 2)}</pre>
        </ScrollArea>
      </DialogBody>
    </Dialog>
  );
}
