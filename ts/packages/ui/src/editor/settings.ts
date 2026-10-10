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
  fontFamily: string;
  indentSize: number;
  insertSpaces: boolean;
  tabSize: number;
  colorPalette: Palette;
};
export const defaultEditorSettings: EditorSettings = {
  fontFamily: "ui-monospace, SFMono-Regular, Consolas, monospace",
  indentSize: 2,
  insertSpaces: true,
  tabSize: 4,
  colorPalette: "muted",
};
export function paletteForTheme(
  name: Palette,
  theme: "light" | "dark" = "dark",
) {
  const palette = palettes[name];
  const result: Record<string, string> = {};
  for (const key of [
    "comment",
    "keyword",
    "title",
    "string",
    "number",
    "attr",
  ] as const) {
    const color = palette[key];
    result[key] =
      theme === "dark"
        ? color
        : "#" +
          [1, 3, 5]
            .map((offset) => {
              const channel = parseInt(color.slice(offset, offset + 2), 16);
              return (
                name === "monochrome"
                  ? Math.max(24, 255 - channel)
                  : Math.round(channel * 0.72)
              )
                .toString(16)
                .padStart(2, "0");
            })
            .join("");
  }
  return result;
}
