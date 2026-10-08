import { defineConfig } from "vite";
import { loadDevGateway, devGatewayPlugin } from "./scripts/web-dev.mjs";

export default defineConfig(async () => ({
  plugins: [devGatewayPlugin(await loadDevGateway())],
  server: { host: "127.0.0.1", port: 5173, strictPort: true },
}));
