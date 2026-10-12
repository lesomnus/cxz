import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "e2e-plaintext",
  testMatch: "multi-window.spec.ts",
  workers: 1,
  use: { ...devices["Desktop Chrome"], baseURL: "http://127.0.0.1:5174" },
  webServer: [
    {
      command:
        "cd .. && CXZ_WEB_FIXTURE=1 CXZ_WEB_FIXTURE_PLAINTEXT=1 TMPDIR=/tmp go test ./internal/webui -run TestBrowserFixture -count=1 -v -timeout 11m",
      url: "http://127.0.0.1:18082",
      timeout: 120000,
    },
    {
      command: "node test-support/vite-gateway-fixture.mjs",
      url: "http://127.0.0.1:5174",
      timeout: 60000,
    },
  ],
  reporter: "list",
});
