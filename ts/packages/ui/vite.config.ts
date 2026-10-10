import { defineConfig } from "vite";
export default defineConfig({
  base: "./",
  build: {
    lib: {
      entry: { index: "src/index.ts", editor: "src/editor/index.ts" },
      formats: ["es"],
      fileName: (_format, entry) => `${entry}.js`,
      cssFileName: "style",
    },
    rollupOptions: {
      external: (id) =>
        /^(react|react-dom)(\/|$)/.test(id) ||
        (id.startsWith("monaco-editor/") && !id.includes("?worker")),
    },
  },
  worker: { format: "es" },
});
