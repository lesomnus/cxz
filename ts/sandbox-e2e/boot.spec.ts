import { test, expect } from "@playwright/test";

// The same viewport the other desktop specs use: the status panel this waits for
// is part of the wide layout.
test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

// The failure this guards against is a boot that never finishes and never says
// anything: the module's response arrives, no progress follows, and the page
// stays on "Starting sandbox…" until whatever is watching gives up. It is rare
// and load-dependent, so it is forced here rather than waited for -- the first
// request for the module is accepted and then abandoned, which is what a stalled
// body read looks like from the page.
test("a stalled module boots on the retry and says what happened", async ({
  page,
}) => {
  let requests = 0;
  await page.route("**/app.wasm", async (route) => {
    requests++;
    if (requests === 1) return; // accepted, never answered
    await route.continue();
  });

  await page.goto("/sandbox.html");
  await expect(page.getByRole("status")).toHaveText(/Starting sandbox/);
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  // The stall is still explained after the recovery: a page that silently
  // recovered teaches nobody why the start took fifteen seconds.
  await expect(page.getByRole("note")).toHaveText(/first attempt stalled/);
  expect(requests).toBe(2);
  // Recovered means usable, not merely rendered.
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(page.locator(".transcript")).toBeVisible();
});

// Two stalls are reported rather than retried forever: an unbounded retry is the
// same hang with more logs.
test("a boot that keeps stalling is reported", async ({ page }) => {
  await page.route("**/app.wasm", async () => {});
  await page.goto("/sandbox.html");
  await expect(page.getByRole("alert")).toHaveText(/did not start within/, {
    timeout: 45000,
  });
  await expect(page.getByRole("alert")).toHaveText(/Reset the sandbox/);
  await expect(page.getByRole("status")).toHaveCount(0);
});
