import { defineConfig } from "vite";
import { noxConfig } from "./scripts/nox-vite.mjs";
export default defineConfig(({ command }) => ({
  ...noxConfig(command),
  build: { outDir: "../internal/webui/assets", emptyOutDir: true },
}));
