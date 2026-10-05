import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "e2e-plaintext",
  workers: 1,
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:18082",
  },
  webServer: {
    command:
      "cd .. && CXZ_WEB_FIXTURE=1 CXZ_WEB_FIXTURE_PLAINTEXT=1 TMPDIR=/tmp go test ./internal/webui -run TestBrowserFixture -count=1 -v -timeout 11m",
    url: "http://127.0.0.1:18082",
    timeout: 120000,
  },
  reporter: "list",
});
