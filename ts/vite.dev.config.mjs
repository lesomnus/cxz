import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { noxConfig, noxDevServer } from "./scripts/nox-vite.mjs";
import { loadDevConnections, devGatewayPlugin } from "./scripts/web-dev.mjs";

export default defineConfig(async () => ({
  ...noxConfig("serve"),
  plugins: [react(), devGatewayPlugin(await loadDevConnections())],
  server: { ...noxDevServer, host: "127.0.0.1", port: 5173, strictPort: true },
}));
