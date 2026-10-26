import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { noxConfig, noxDevServer } from "./scripts/nox-vite.mjs";
const headers = {
  "Cross-Origin-Opener-Policy": "same-origin",
  "Cross-Origin-Embedder-Policy": "require-corp",
};
export default defineConfig(({ command }) => ({
  ...noxConfig(command),
  plugins: command === "serve" ? [react()] : [],
  publicDir: ".sandbox",
  optimizeDeps: {
    exclude: [
      "@lesomnus/grpc-dgram",
      "@lesomnus/nox",
      "@lesomnus/nox/editor",
      "@lesomnus/nox/terminal",
    ],
  },
  server: {
    ...noxDevServer,
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    headers,
  },
  preview: { host: "127.0.0.1", port: 5173, strictPort: true, headers },
  // grpc-dgram declares sideEffects:false, but its worker entry self-registers.
  // Keep that registration in the emitted worker bundle.
  worker: { format: "es", rollupOptions: { treeshake: false } },
  build: {
    outDir: "dist-sandbox",
    emptyOutDir: true,
    rollupOptions: { input: "sandbox.html" },
  },
}));
