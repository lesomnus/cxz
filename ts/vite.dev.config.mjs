import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { noxConfig, noxDevServer } from "./scripts/nox-vite.mjs";
import { loadDevGateway, devGatewayPlugin } from "./scripts/web-dev.mjs";

// Served on every interface, so the machine cxz runs on is not the only machine
// this can be read from -- a workstation on the same network opens
// http://<address>:5173 and talks to the installation. `CXZ_DEV_HOST=127.0.0.1`
// puts it back on loopback alone.
//
// What that hands out is the point, not a side effect: this server signs its
// browser in as the installation owner with no credential of its own (see
// web-dev.mjs), so anybody who can reach the port is the owner. It stays an
// address-or-listed-name check rather than opening up to any Host header --
// `server.allowedHosts` is where a name goes, and it is the same list Vite
// checks for the page itself.
const host = process.env.CXZ_DEV_HOST ?? "0.0.0.0";

export default defineConfig(async () => ({
  ...noxConfig("serve"),
  plugins: [react(), devGatewayPlugin(await loadDevGateway())],
  server: { ...noxDevServer, host, port: 5173, strictPort: true },
}));
