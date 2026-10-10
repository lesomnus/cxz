// Monaco 0.55.1's ESM JSON contribution ships an empty declaration, although
// its runtime exports jsonDefaults. Reuse the package's public JSON type.
declare module "monaco-editor/esm/vs/language/json/monaco.contribution.js" {
  export const jsonDefaults: typeof import("monaco-editor").json.jsonDefaults;
}
