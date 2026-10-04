import { defineConfig } from "vite";
const headers = {
  "Cross-Origin-Opener-Policy": "same-origin",
  "Cross-Origin-Embedder-Policy": "require-corp",
};
export default defineConfig({
  publicDir: ".sandbox",
  optimizeDeps: { exclude: ["@lesomnus/grpc-dgram"] },
  server: { host: "127.0.0.1", port: 5173, strictPort: true, headers },
  preview: { host: "127.0.0.1", port: 5173, strictPort: true, headers },
  worker: { format: "es", rollupOptions: { treeshake: false } },
  build: {
    outDir: "dist-sandbox",
    emptyOutDir: true,
    rollupOptions: { input: "sandbox.html" },
  },
});
