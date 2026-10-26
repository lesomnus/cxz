import { useEffect, useState } from "react";
import { CompactNumber, Popover } from "@lesomnus/nox";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import type { FileSizeReader } from "./file-size";
import styles from "./file-link-hint.module.css";

export function FileLinkHint({
  anchor,
  path,
  readSize,
  close,
}: {
  anchor: HTMLAnchorElement;
  path: string;
  readSize?: FileSizeReader;
  close: () => void;
}) {
  const { locale } = useLocale();
  const [size, setSize] = useState<bigint>();
  const [failed, setFailed] = useState(!readSize);
  useEffect(() => {
    if (!readSize) return;
    const controller = new AbortController();
    void readSize(path, controller.signal).then(
      (value) => {
        if (!controller.signal.aborted) setSize(value);
      },
      () => {
        if (!controller.signal.aborted) setFailed(true);
      },
    );
    return () => controller.abort();
  }, [path, readSize]);
  return (
    <Popover
      anchor={anchor}
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
      content={
        <span className={styles.content}>
          <span
            className={styles.size}
            aria-busy={size === undefined && !failed}
          >
            <CompactNumber
              value={size === undefined ? NaN : Number(size)}
              label={
                size !== undefined
                  ? t("{size} bytes", { size: size.toLocaleString(locale) })
                  : failed
                    ? t("Size unavailable")
                    : t("Loading…")
              }
            />
            <span className={styles.unit} aria-hidden="true">
              B
            </span>
          </span>
          <span className={styles.path}>{path}</span>
        </span>
      }
    />
  );
}
