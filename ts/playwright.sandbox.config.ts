import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "sandbox-e2e",
  // One worker, deliberately. These specs assert on motion -- holds,
  // transitions, elastic settling -- and motion is measured in wall clock, so
  // workers compete for frame deadlines rather than for idle cores. Measured on
  // a 16-core machine: four workers cut the wall clock 344s to 205s while the
  // time spent inside tests grew 337s to 751s, and three tests timed out that
  // pass serially. One spec went from 9.8s to 63.7s.
  workers: 1,
  // A test that waits for motion is slow by nature, and a loaded machine makes
  // it slower. Waiting longer costs nothing when the assertion holds; what it
  // buys is that a slow machine reports a real failure instead of a deadline.
  timeout: 120000,
  expect: { timeout: 15000 },
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
