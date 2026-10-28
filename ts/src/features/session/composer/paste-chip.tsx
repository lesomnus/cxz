import { useEffect, useMemo, useState } from "react";
import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import {
  CompactNumber,
  FileChip,
  Popover,
  SegmentedControl,
} from "@lesomnus/nox";
import {
  isTextPaste,
  pasteSendMode,
  type ComposerPaste,
} from "./composer-pastes";
import styles from "./paste-chip.module.css";
import { presentPaste, type PastePresentation } from "./paste-presentation";
import { pasteExcerpt } from "./paste-chip-layout";

export function PasteChip({
  paste,
  onOpen,
  onSelect,
  onSendModeChange,
  canSendFile = false,
  readOnly = false,
  presentation = presentPaste(paste),
}: {
  paste: ComposerPaste;
  onOpen: () => void;
  onSelect?: () => void;
  onSendModeChange?: (mode: "text" | "file") => void;
  canSendFile?: boolean;
  readOnly?: boolean;
  presentation?: PastePresentation;
}) {
  const attachment = paste.attachment;
  const textPaste = isTextPaste(paste);
  const mode = pasteSendMode(paste);
  const bytes = `${paste.bytes.toLocaleString(currentLocale())} B`;
  const { label, before } = presentation;
  const [anchor, setAnchor] = useState<HTMLSpanElement | null>(null);
  const [open, setOpen] = useState(false);
  const preview = useMemo(() => pasteExcerpt(paste.body), [paste.body]);
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    document.addEventListener("keydown", close, true);
    return () => document.removeEventListener("keydown", close, true);
  }, [open]);
  return (
    <>
      <FileChip
        ref={textPaste ? setAnchor : undefined}
        className={`paste-chip ${styles.chip}`}
        textLayout
        textPrefix={presentation.prefix}
        leading={
          textPaste ? (
            <SegmentedControl
              label={t("Send pasted content as")}
              size="inline"
              iconOnly
              value={mode}
              disabled={readOnly || !onSendModeChange}
              options={[
                { value: "text", label: t("Text"), icon: <TextModeIcon /> },
                {
                  value: "file",
                  label: t("File"),
                  icon: <FileModeIcon />,
                  disabled: !canSendFile && !attachment?.path,
                },
              ]}
              onChange={(value) => onSendModeChange?.(value as "text" | "file")}
            />
          ) : undefined
        }
        loading={
          attachment?.state === "uploading" && (!textPaste || mode === "file")
        }
        data-upload-state={attachment?.state}
        data-paste-token={paste.token}
        data-send-as={textPaste ? mode : undefined}
        role={textPaste ? "group" : "button"}
        tabIndex={textPaste ? undefined : 0}
        aria-label={
          paste.secret
            ? "Secret file reference"
            : textPaste
              ? t("Pasted text")
              : label.trim()
        }
        aria-description={textPaste ? bytes : undefined}
        title={
          textPaste
            ? undefined
            : (attachment?.error ??
              (attachment?.state === "uploading"
                ? t("Uploading…")
                : `${bytes} · ${t("View source · Ctrl+P")}`))
        }
        onPointerEnter={(event) => {
          if (textPaste && event.pointerType !== "touch") setOpen(true);
        }}
        onPointerLeave={() => setOpen(false)}
        onFocusCapture={() => {
          if (textPaste) setOpen(true);
        }}
        onBlurCapture={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null))
            setOpen(false);
        }}
        onClick={(event) => {
          if (textPaste) {
            setOpen(false);
            if (!(event.target as Element).closest("[data-file-chip-control]"))
              onSelect?.();
          } else onOpen();
        }}
        onKeyDown={(event) => {
          if (textPaste) {
            if (event.key === "Enter") {
              event.preventDefault();
              event.stopPropagation();
            }
          } else if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onOpen();
          }
        }}
      >
        <span className="paste-chip-label" aria-hidden="true">
          {presentation.progress !== undefined ? (
            <>
              {before}
              <span className={styles.progress}>
                {" "}
                <CompactNumber
                  className={styles.number}
                  value={presentation.progress}
                  mode="integer"
                  digits={3}
                  muted
                />
                %
              </span>
            </>
          ) : presentation.bytes !== undefined ? (
            <>
              {before}
              <CompactNumber
                className={styles.number}
                value={presentation.bytes}
                baseUnit="B"
                label={bytes}
              />
            </>
          ) : (
            label
          )}
        </span>
      </FileChip>
      {textPaste && (
        <Popover
          anchor={anchor}
          open={open}
          onOpenChange={setOpen}
          content={preview}
          placement="top-start"
        />
      )}
    </>
  );
}

function TextModeIcon() {
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M4 5h16M12 5v14M8 19h8" />
    </svg>
  );
}
function FileModeIcon() {
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9Z" />
      <path d="M14 3v6h6" />
    </svg>
  );
}
