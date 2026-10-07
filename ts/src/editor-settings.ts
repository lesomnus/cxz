import { t } from "./i18n";
export const palettes = {
  muted: {
    label: "Muted",
    comment: "#697b6d",
    keyword: "#987fa8",
    title: "#7797ac",
    string: "#839c7b",
    number: "#ad8d6b",
    attr: "#a18b7c",
  },
  monochrome: {
    label: "Monochrome",
    comment: "#808080",
    keyword: "#ededed",
    title: "#ededed",
    string: "#bdbdbd",
    number: "#bdbdbd",
    attr: "#bdbdbd",
  },
  cool: {
    label: "Cool",
    comment: "#657b85",
    keyword: "#7e8faf",
    title: "#71a0ab",
    string: "#7b9d91",
    number: "#9b89b0",
    attr: "#899baa",
  },
  warm: {
    label: "Warm",
    comment: "#80786b",
    keyword: "#aa838e",
    title: "#a6977b",
    string: "#91996f",
    number: "#b28e6e",
    attr: "#a48b79",
  },
} as const;
export type Palette = keyof typeof palettes;
export type EditorSettings = {
  indentSize: number;
  insertSpaces: boolean;
  tabSize: number;
  colorPalette: Palette;
};
export const defaultEditorSettings: EditorSettings = {
  indentSize: 2,
  insertSpaces: true,
  tabSize: 4,
  colorPalette: "muted",
};
export type SettingsDocument = Record<string, unknown>;
export type EditorScope = "global" | "session";
export const editorKeys = [
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
    if (Object.hasOwn(document, name))
      Object.assign(result, { [key]: document[name] });
  }
  return result;
}

export function paletteVariables(name: Palette) {
  const palette = palettes[name];
  return Object.fromEntries(
    ["comment", "keyword", "title", "string", "number", "attr"].map((key) => [
      `--code-${key}`,
      palette[key as keyof typeof palette],
    ]),
  );
}
