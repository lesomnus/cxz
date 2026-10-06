import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import "monaco-editor/esm/vs/basic-languages/go/go.contribution.js";
import "monaco-editor/esm/vs/basic-languages/typescript/typescript.contribution.js";
import "monaco-editor/esm/vs/basic-languages/javascript/javascript.contribution.js";
import "monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution.js";
import "monaco-editor/esm/vs/basic-languages/shell/shell.contribution.js";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker.js?worker";

(
  globalThis as typeof globalThis & { MonacoEnvironment: unknown }
).MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
};
monaco.editor.defineTheme("cxz", {
  base: "vs-dark",
  inherit: false,
  rules: [
    { token: "", foreground: "EDEDED" },
    { token: "comment", foreground: "808080" },
    { token: "keyword", foreground: "FFFFFF" },
    { token: "string", foreground: "BDBDBD" },
    { token: "number", foreground: "BDBDBD" },
  ],
  colors: {
    "editor.background": "#141414",
    "editor.foreground": "#ededed",
    "editorLineNumber.foreground": "#686868",
    "editorLineNumber.activeForeground": "#bdbdbd",
    "editor.selectionBackground": "#414141",
    "editor.inactiveSelectionBackground": "#303030",
    "editor.lineHighlightBackground": "#1b1b1b",
    "editorCursor.foreground": "#ededed",
    "editorWidget.background": "#222222",
    "editorWidget.border": "#363636",
    focusBorder: "#737373",
  },
});
export { monaco };

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
