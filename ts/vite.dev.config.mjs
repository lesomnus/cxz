import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { noxConfig } from "./scripts/nox-vite.mjs";
import { loadDevGateway, devGatewayPlugin } from "./scripts/web-dev.mjs";

export default defineConfig(async () => ({
  ...noxConfig("serve"),
  plugins: [react(), devGatewayPlugin(await loadDevGateway())],
  server: { host: "127.0.0.1", port: 5173, strictPort: true },
}));
