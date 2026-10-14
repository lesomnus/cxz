import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "performance-e2e",
  workers: 1,
  timeout: 60000,
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:6013",
    launchOptions: {
      args: [
        "--no-zygote",
        "--disable-gpu",
        "--renderer-process-limit=1",
        "--num-raster-threads=1",
      ],
    },
  },
  webServer: {
    command:
      "node node_modules/vite/bin/vite.js --config vite.config.ts --port 6013 --host 127.0.0.1 --strictPort",
    url: "http://127.0.0.1:6013/test-support/transcript-performance.html",
    timeout: 30000,
  },
  reporter: "list",
});
