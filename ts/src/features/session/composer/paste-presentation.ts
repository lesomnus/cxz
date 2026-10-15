import { formatCompactNumber } from "@lesomnus/nox";
import { currentLocale, t } from "#src/shared/i18n/i18n.ts";
import type { ComposerPaste } from "./composer-pastes";

export type PastePresentation = {
  label: string;
  before: string;
  leading: string;
  after: string;
};

export function presentPaste(paste: ComposerPaste): PastePresentation {
  const attachment = paste.attachment;
  const text = !paste.secret && (!attachment || !!paste.body);
  if (text && attachment?.state === "uploading") {
    const progress = Math.max(
      0,
      Math.min(100, Math.floor(attachment.progress ?? 0)),
    );
    const digits = String(progress);
    const leading = "0".repeat(3 - digits.length);
    return {
      label: `[Paste ${leading}${digits}%]`,
      before: "[Paste ",
      leading,
      after: `${digits}%]`,
    };
  }
  if (text && attachment?.state !== "error") {
    const size = formatCompactNumber(paste.bytes);
    const after = size.digits + (size.unit === "·" ? "B" : size.unit) + "]";
    return {
      label: `[Text ${size.leading}${after}`,
      before: "[Text ",
      leading: size.leading,
      after,
    };
  }
  const status =
    attachment?.state === "uploading"
      ? t("Uploading…")
      : attachment?.state === "error"
        ? t("Upload failed.")
        : `${paste.bytes.toLocaleString(currentLocale())} B`;
  const label = paste.secret
    ? paste.token
    : text
      ? `[Paste ${t("Failed")}]`
      : `${attachment!.name} · ${status}`;
  return { label, before: label, leading: "", after: "" };
}
