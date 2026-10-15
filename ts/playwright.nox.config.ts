import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "nox-e2e",
  workers: 1,
  timeout: 30000,
  use: { ...devices["Desktop Chrome"], baseURL: "http://127.0.0.1:6014" },
  webServer: {
    command:
      "node node_modules/vite/bin/vite.js --config vite.nox-development.config.ts",
    url: "http://127.0.0.1:6014/test-support/nox-development.html",
    timeout: 30000,
  },
});
