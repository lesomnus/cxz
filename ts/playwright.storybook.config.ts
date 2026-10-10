import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "storybook-e2e",
  workers: 1,
  timeout: 30000,
  expect: { timeout: 10000 },
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:6007",
    screenshot: "only-on-failure",
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
  // Exercise the deployable Storybook, independently of the user's Vite server.
  webServer: {
    command:
      "node node_modules/vite/bin/vite.js preview --outDir storybook-static --port 6007 --host 127.0.0.1 --strictPort",
    url: "http://127.0.0.1:6007/iframe.html",
    timeout: 30000,
  },
  reporter: "list",
});
