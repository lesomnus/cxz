import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import { FileChip } from "@lesomnus/nox";
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
  const bytes = `${paste.bytes.toLocaleString(currentLocale())} B`;
  const { label, before, leading, after } = presentation;
  return (
    <FileChip
      className={`paste-chip ${styles.chip}`}
      textLayout
      loading={attachment?.state === "uploading"}
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
      <span className="paste-chip-label" aria-hidden="true">
        {before}
        <span className={styles.leading} data-paste-leading>
          {leading}
        </span>
        {after}
      </span>
    </FileChip>
  );
}
