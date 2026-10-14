import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { noxConfig } from "./scripts/nox-vite.mjs";
export default defineConfig({
  ...noxConfig("serve"),
  plugins: [react()],
  server: { host: "127.0.0.1", port: 6014, strictPort: true },
});
