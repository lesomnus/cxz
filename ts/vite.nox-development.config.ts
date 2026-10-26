import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { noxConfig, noxDevServer } from "./scripts/nox-vite.mjs";
export default defineConfig({
  ...noxConfig("serve"),
  plugins: [react()],
  server: { ...noxDevServer, host: "127.0.0.1", port: 6014, strictPort: true },
});
