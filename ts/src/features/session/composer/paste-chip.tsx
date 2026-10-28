import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import type { ComposerPaste } from "./composer-pastes";
import styles from "./paste-chip.module.css";
import { presentPaste, type PastePresentation } from "./paste-presentation";

export function PasteChip({
  paste,
  onOpen,
  presentation = presentPaste(paste),
}: {
  paste: ComposerPaste;
  onOpen: () => void;
  presentation?: PastePresentation;
}) {
  const attachment = paste.attachment;
  const state = paste.secret ?? attachment;
  const bytes = `${paste.bytes.toLocaleString(currentLocale())} B`;
  const { label, before, leading, after } = presentation;
  return (
    <span
      className={`paste-chip ${styles.chip}`}
      data-upload-state={state?.state}
      aria-busy={state?.state === "uploading"}
      role="button"
      tabIndex={0}
      aria-label={paste.secret ? "Secret file reference" : label}
      title={
        paste.secret
          ? (paste.secret.error ??
            `${paste.secret.kind ?? "Secret"} · ${paste.secret.state === "uploading" ? "Preparing secret file…" : "Secret file reference"}`)
          : (attachment?.error ??
            (attachment?.state === "uploading"
              ? t("Uploading…")
              : `${bytes} · ${t("View source · Ctrl+P")}`))
      }
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
    >
      <span className={`paste-chip-label ${styles.label}`} aria-hidden="true">
        {before}
        <span className={styles.leading} data-paste-leading>
          {leading}
        </span>
        {after}
      </span>
    </span>
  );
}
