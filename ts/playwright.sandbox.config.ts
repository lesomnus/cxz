import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "sandbox-e2e",
  // One worker, deliberately. These specs assert on motion -- holds,
  // transitions, elastic settling -- and motion is measured in wall clock, so
  // workers compete for frame deadlines rather than for idle cores. A worker is
  // already six processes and eighty threads at 2.8 of sixteen cores, so the
  // cores are not what runs out: a missed 16.7ms deadline makes the motion a
  // test is waiting for take longer.
  //
  // Measured on sixteen cores: four workers cut the wall clock from 378s to
  // 222s, while the time spent inside tests grew from 372s to 624s and the
  // suite went from 86 passing to 58 passing and 17 failing. pinned-prompt
  // alone went from 30.0s to 68.5s. The wall clock is not worth that.
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
