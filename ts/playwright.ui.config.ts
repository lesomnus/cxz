import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "ui-e2e",
  workers: 1,
  timeout: 30000,
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:6009",
    trace: "retain-on-failure",
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
      "node node_modules/vite/bin/vite.js preview --outDir storybook-ui-static --host 127.0.0.1 --port 6009 --strictPort",
    url: "http://127.0.0.1:6009/iframe.html",
    timeout: 30000,
  },
});
