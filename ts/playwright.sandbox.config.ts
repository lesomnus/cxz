import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "sandbox-e2e",
  workers: 1,
  timeout: 60000,
  use: {
    ...devices["iPhone 13"],
    defaultBrowserType: "chromium",
    baseURL: "http://127.0.0.1:5173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "npx vite preview --config vite.sandbox.config.ts",
    url: "http://127.0.0.1:5173/sandbox.html",
    timeout: 60000,
  },
  reporter: "list",
});
