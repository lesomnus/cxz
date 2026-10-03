import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "e2e",
  workers: 1,
  use: {
    ...devices["iPhone 13"],
    defaultBrowserType: "chromium",
    baseURL: "https://127.0.0.1:18081",
    ignoreHTTPSErrors: true,
  },
  webServer: {
    command:
      "cd .. && CXZ_WEB_FIXTURE=1 TMPDIR=/tmp go test ./internal/webui -run TestBrowserFixture -count=1 -v -timeout 11m",
    url: "https://127.0.0.1:18081",
    ignoreHTTPSErrors: true,
    timeout: 120000,
  },
  reporter: "list",
});
