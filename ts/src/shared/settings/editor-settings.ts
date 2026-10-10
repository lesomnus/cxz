import { t } from "#src/shared/i18n/i18n.ts";
import {
  palettes,
  paletteForTheme,
  defaultEditorSettings as defaultUIEditorSettings,
  type EditorSettings as UIEditorSettings,
} from "@lesomnus/cxz-ui/editor";
export { palettes, paletteForTheme } from "@lesomnus/cxz-ui/editor";
export type { Palette } from "@lesomnus/cxz-ui/editor";
import type { Palette } from "@lesomnus/cxz-ui/editor";
export type GoogleFont = { provider: "google"; family: string };
export type FontFamilySetting = string | GoogleFont;
export function isGoogleFont(value: unknown): value is GoogleFont {
  if (!value || typeof value !== "object") return false;
  const font = value as GoogleFont;
  return (
    font.provider === "google" &&
    typeof font.family === "string" &&
    font.family === font.family.trim() &&
    font.family.length > 0 &&
    font.family.length <= 100 &&
    /^[\p{L}\p{N} -]+$/u.test(font.family)
  );
}
export function fontFamilyCSS(value: FontFamilySetting): string {
  return typeof value === "string"
    ? value
    : `${JSON.stringify(value.family)}, monospace`;
}
export type EditorSettings = UIEditorSettings & { googleFont?: string };
export const defaultEditorSettings: EditorSettings = {
  ...defaultUIEditorSettings,
};
export type SettingsDocument = Record<string, unknown>;
export type EditorScope = "global" | "session";
export const editorKeys = [
  "fontFamily",
  "indentSize",
  "insertSpaces",
  "tabSize",
  "colorPalette",
] as const;
export const EMPTY_SETTINGS = "{}\n";
export const MAX_SETTINGS_BYTES = 1024 * 1024;

export function parseSettings(raw: string): SettingsDocument {
  if (new TextEncoder().encode(raw).length > MAX_SETTINGS_BYTES)
    throw new Error(t("Settings files are limited to 1 MiB."));
  const document: unknown = JSON.parse(raw);
  if (!document || typeof document !== "object" || Array.isArray(document))
    throw new Error(t("Settings must be a JSON object."));
  const value = document as SettingsDocument;
  for (const prefix of ["editor.", "session.editor."]) {
    for (const key of editorKeys) {
      const name = prefix + key;
      if (!Object.hasOwn(value, name)) continue;
      const entry = value[name];
      if (key === "indentSize" || key === "tabSize") {
        if (
          typeof entry !== "number" ||
          !Number.isInteger(entry) ||
          entry < 1 ||
          entry > 16
        )
          throw new Error(
            t("{name}: Enter an integer between 1 and 16.", { name }),
          );
      } else if (key === "fontFamily") {
        if (
          !isGoogleFont(entry) &&
          (typeof entry !== "string" || !entry.trim() || entry.length > 1024)
        )
          throw new Error(
            t("{name}: Enter a font family list or a Google Fonts selection.", {
              name,
            }),
          );
      } else if (key === "insertSpaces") {
        if (typeof entry !== "boolean")
          throw new Error(t("{name}: Enter true or false.", { name }));
      } else if (typeof entry !== "string" || !Object.hasOwn(palettes, entry))
        throw new Error(
          t("{name}: Choose a supported color palette.", { name }),
        );
    }
  }
  if (
    Object.hasOwn(value, "ui.language") &&
    value["ui.language"] !== "en" &&
    value["ui.language"] !== "ko"
  )
    throw new Error(t("Unsupported display language. Choose en or ko."));
  if (
    Object.hasOwn(value, "ui.theme") &&
    !["light", "dark", "system"].includes(value["ui.theme"] as string)
  )
    throw new Error(t("Unsupported theme. Choose light, dark or system."));
  return value;
}

export function resolveEditorSettings(
  document: SettingsDocument,
  scope: EditorScope = "global",
): EditorSettings {
  const result = { ...defaultEditorSettings };
  for (const key of editorKeys) {
    const scoped = `session.editor.${key}`;
    const global = `editor.${key}`;
    const name =
      scope === "session" && Object.hasOwn(document, scoped) ? scoped : global;
    if (Object.hasOwn(document, name)) {
      if (key === "fontFamily") {
        const font = document[name] as FontFamilySetting;
        result.fontFamily = fontFamilyCSS(font);
        if (isGoogleFont(font)) result.googleFont = font.family;
      } else Object.assign(result, { [key]: document[name] });
    }
  }
  return result;
}

export function paletteVariables(
  name: Palette,
  theme: "light" | "dark" = "dark",
) {
  const palette = paletteForTheme(name, theme);
  return Object.fromEntries(
    ["comment", "keyword", "title", "string", "number", "attr"].map((key) => [
      `--code-${key}`,
      palette[key as keyof typeof palette],
    ]),
  );
}
