import { formatCompactNumber } from "@lesomnus/cxz-ui";
import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import type { ComposerPaste } from "./composer-pastes";
import styles from "./paste-chip.module.css";

export function PasteChip({
  paste,
  onOpen,
}: {
  paste: ComposerPaste;
  onOpen: () => void;
}) {
  const attachment = paste.attachment;
  const text = !paste.secret && (!attachment || !!paste.body);
  const progress = Math.max(
    0,
    Math.min(100, Math.floor(attachment?.progress ?? 0)),
  );
  const digits = String(progress);
  const leading = "0".repeat(3 - digits.length);
  const size = formatCompactNumber(paste.bytes);
  const compactSize = size.digits + (size.unit === "·" ? "B" : size.unit);
  const bytes = `${paste.bytes.toLocaleString(currentLocale())} B`;
  const status =
    attachment?.state === "uploading"
      ? t("Uploading…")
      : attachment?.state === "error"
        ? t("Upload failed.")
        : bytes;
  const label = paste.secret
    ? paste.token
    : text
      ? attachment?.state === "uploading"
        ? `[Paste ${leading}${digits}%]`
        : attachment?.state === "error"
          ? `[Paste ${t("Failed")}]`
          : `[Text ${compactSize}]`
      : `${attachment!.name} · ${status}`;
  return (
    <span
      className={`paste-chip ${styles.chip}`}
      data-upload-state={attachment?.state}
      role="button"
      tabIndex={0}
      aria-label={paste.secret ? "Secret file reference" : label}
      title={
        attachment?.error ??
        (attachment?.state === "uploading"
          ? t("Uploading…")
          : `${bytes} · ${t("View source · Ctrl+P")}`)
      }
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
    >
      {/* Match native textarea geometry; only the overlay changes on progress. */}
      <span className={`paste-chip-token ${styles.token}`} aria-hidden="true">
        {paste.token}
      </span>
      <span className={`paste-chip-label ${styles.label}`} aria-hidden="true">
        <span>
          {text && attachment?.state === "uploading" ? (
            <>
              {"[Paste "}
              <span className={styles.leading} data-paste-leading>
                {leading}
              </span>
              {digits}%]
            </>
          ) : (
            label
          )}
        </span>
      </span>
    </span>
  );
}
