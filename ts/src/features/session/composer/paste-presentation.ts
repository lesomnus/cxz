import { formatCompactNumber } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import {
  isTextPaste,
  pasteSendMode,
  type ComposerPaste,
} from "./composer-pastes";
import { PASTE_CHIP_PREFIX } from "./paste-chip-layout";

export type PastePresentation = {
  label: string;
  before: string;
  leading: string;
  after: string;
  bytes?: number;
  progress?: number;
  prefix?: string;
  layoutText?: string;
};

export function presentPaste(paste: ComposerPaste): PastePresentation {
  const attachment = paste.attachment;
  const text = isTextPaste(paste) || (!paste.secret && !attachment);
  if (paste.secret)
    return { label: paste.token, before: paste.token, leading: "", after: "" };
  const before = text ? "" : `${attachment!.name} · `;
  const prefix = text ? PASTE_CHIP_PREFIX : undefined;
  const asText = text && pasteSendMode(paste) === "text";
  if (!asText && attachment?.state === "uploading") {
    const progress = Math.max(
      0,
      Math.min(100, Math.floor(attachment.progress ?? 0)),
    );
    const digits = String(progress);
    const leading = "0".repeat(3 - digits.length);
    return {
      label: `${before} ${leading}${digits}%`,
      before,
      leading,
      after: `${digits}%`,
      progress,
      prefix,
    };
  }
  if (asText || attachment?.state !== "error") {
    const size = formatCompactNumber(paste.bytes);
    const after = size.digits + (size.unit === "·" ? "B" : size.unit);
    return {
      label: before + size.leading + after,
      before,
      leading: size.leading,
      after,
      bytes: paste.bytes,
      prefix,
    };
  }
  const label = before + t("Upload failed.");
  return { label, before: label, leading: "", after: "", prefix };
}
