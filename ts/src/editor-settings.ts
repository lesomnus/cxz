export const palettes = {
  muted: {
    label: "차분한 색상",
    comment: "#697b6d",
    keyword: "#987fa8",
    title: "#7797ac",
    string: "#839c7b",
    number: "#ad8d6b",
    attr: "#a18b7c",
  },
  monochrome: {
    label: "흑백",
    comment: "#808080",
    keyword: "#ededed",
    title: "#ededed",
    string: "#bdbdbd",
    number: "#bdbdbd",
    attr: "#bdbdbd",
  },
  cool: {
    label: "차가운 색상",
    comment: "#657b85",
    keyword: "#7e8faf",
    title: "#71a0ab",
    string: "#7b9d91",
    number: "#9b89b0",
    attr: "#899baa",
  },
  warm: {
    label: "따뜻한 색상",
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
    throw new Error("설정 파일은 1 MiB까지 저장할 수 있습니다.");
  const document: unknown = JSON.parse(raw);
  if (!document || typeof document !== "object" || Array.isArray(document))
    throw new Error("settings는 JSON 객체여야 합니다.");
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
          throw new Error(`${name}: 1–16 사이의 정수를 입력하세요.`);
      } else if (key === "insertSpaces") {
        if (typeof entry !== "boolean")
          throw new Error(`${name}: true 또는 false를 입력하세요.`);
      } else if (typeof entry !== "string" || !Object.hasOwn(palettes, entry))
        throw new Error(`${name}: 지원하는 색상 팔레트를 선택하세요.`);
    }
  }
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
