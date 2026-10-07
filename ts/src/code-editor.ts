import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import "monaco-editor/esm/vs/basic-languages/go/go.contribution.js";
import "monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution.js";
import "monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution.js";
import "monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution.js";
import "monaco-editor/esm/vs/basic-languages/shell/shell.contribution.js";
import { jsonDefaults } from "monaco-editor/esm/vs/language/json/monaco.contribution.js";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker.js?worker";
import JsonWorker from "monaco-editor/esm/vs/language/json/json.worker.js?worker";
import {
  paletteForTheme,
  palettes,
  type EditorSettings,
} from "./editor-settings";

(
  globalThis as typeof globalThis & { MonacoEnvironment: unknown }
).MonacoEnvironment = {
  getWorker: (_module: string, label: string) =>
    label === "json" ? new JsonWorker() : new EditorWorker(),
};
jsonDefaults.setDiagnosticsOptions({
  validate: true,
  allowComments: false,
  enableSchemaRequest: false,
});
export const editorOptions: monaco.editor.IStandaloneEditorConstructionOptions =
  {
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 12,
    fontFamily: "ui-monospace, SFMono-Regular, Consolas, monospace",
    scrollBeyondLastLine: false,
    padding: { top: 8, bottom: 8 },
    renderLineHighlight: "none",
    overviewRulerLanes: 0,
    hideCursorInOverviewRuler: true,
    contextmenu: false,
    stickyScroll: { enabled: false },
    editContext: false,
  };
for (const name of Object.keys(palettes) as (keyof typeof palettes)[])
  for (const theme of ["dark", "light"] as const) {
    const palette = paletteForTheme(name, theme);
    const light = theme === "light";
    monaco.editor.defineTheme(`cxz-${name}-${theme}`, {
      base: light ? "vs" : "vs-dark",
      inherit: false,
      rules: [
        { token: "", foreground: light ? "202020" : "EDEDED" },
        { token: "comment", foreground: palette.comment.slice(1) },
        { token: "keyword", foreground: palette.keyword.slice(1) },
        { token: "string", foreground: palette.string.slice(1) },
        { token: "string.key.json", foreground: palette.attr.slice(1) },
        { token: "number", foreground: palette.number.slice(1) },
        { token: "type", foreground: palette.title.slice(1) },
        { token: "identifier", foreground: palette.title.slice(1) },
      ],
      colors: {
        "editor.background": light ? "#fafafa" : "#141414",
        "editor.foreground": light ? "#202020" : "#ededed",
        "editorLineNumber.foreground": light ? "#777777" : "#686868",
        "editorLineNumber.activeForeground": light ? "#424242" : "#bdbdbd",
        "editor.selectionBackground": light ? "#bebebe" : "#414141",
        "editor.inactiveSelectionBackground": light ? "#dfdfdf" : "#303030",
        "editor.lineHighlightBackground": light ? "#f5f5f5" : "#1b1b1b",
        "editorCursor.foreground": light ? "#202020" : "#ededed",
        "editorWidget.background": light ? "#eeeeee" : "#222222",
        "editorWidget.border": light ? "#c9c9c9" : "#363636",
        focusBorder: light ? "#8c8c8c" : "#737373",
      },
    });
  }
export { monaco };
export function configureEditor(
  editor: monaco.editor.IStandaloneCodeEditor,
  model: monaco.editor.ITextModel,
  settings: EditorSettings,
  theme: "light" | "dark" = "dark",
) {
  model.updateOptions({
    tabSize: settings.tabSize,
    indentSize: settings.indentSize,
    insertSpaces: settings.insertSpaces,
  });
  editor.updateOptions({ theme: `cxz-${settings.colorPalette}-${theme}` });
}

export function language(path: string) {
  const ext = path.split(".").at(-1)?.toLowerCase();
  return (
    (
      {
        go: "go",
        ts: "typescript",
        tsx: "typescript",
        js: "javascript",
        jsx: "javascript",
        md: "markdown",
        sh: "shell",
        json: "json",
        css: "css",
        html: "html",
      } as Record<string, string>
    )[ext ?? ""] ?? "plaintext"
  );
}
