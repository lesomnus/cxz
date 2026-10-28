import { formatCompactNumber } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import type { ComposerPaste } from "./composer-pastes";

export type PastePresentation = {
  label: string;
  before: string;
  leading: string;
  after: string;
  bytes?: number;
  progress?: number;
};

export function presentPaste(paste: ComposerPaste): PastePresentation {
  const attachment = paste.attachment;
  const text = !paste.secret && (!attachment || !!paste.body);
  if (paste.secret)
    return { label: paste.token, before: paste.token, leading: "", after: "" };
  const before = text ? "" : `${attachment!.name} · `;
  if (attachment?.state === "uploading") {
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
    };
  }
  if (attachment?.state !== "error") {
    const size = formatCompactNumber(paste.bytes);
    const after = size.digits + (size.unit === "·" ? "B" : size.unit);
    return {
      label: before + size.leading + after,
      before,
      leading: size.leading,
      after,
      bytes: paste.bytes,
    };
  }
  const label = before + t("Upload failed.");
  return { label, before: label, leading: "", after: "" };
}
