import { expect, test } from "@playwright/test";

test("sessions requiring Resume are dimmed without stale activity indicators", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--lifecycle-states&viewMode=story",
  );
  for (const state of ["stopped", "interrupted", "failed"]) {
    const row = page.locator(`.tree-session[data-state="${state}"]`);
    await expect(row.locator(".session-heading")).toHaveCSS("opacity", "0.25");
    await expect(row.locator(".session-activity-dot")).toHaveCount(0);
  }
  // A fatal/error notification must remain visible even when the run is gone.
  await expect(
    page
      .locator('.tree-session[data-state="interrupted"]')
      .getByTitle("Unread error"),
  ).toBeVisible();
  for (const state of ["idle", "working"]) {
    await expect(
      page.locator(`.tree-session[data-state="${state}"] .session-heading`),
    ).toHaveCSS("opacity", "1");
  }
  // Completion notifications survive for live, idle sessions.
  await expect(
    page
      .locator('.tree-session[data-state="idle"]')
      .getByTitle("Unread response"),
  ).toBeVisible();
});

test("idle background work uses dim working dots while unread completion takes priority", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--background-only&viewMode=story",
  );
  const background = page.getByRole("img", {
    name: "Background tasks",
    exact: true,
  });
  await expect(background.locator("circle")).toHaveCount(8);
  await expect(background.locator("svg")).toHaveCSS("opacity", "0.4");
  await expect(
    page.getByRole("img", { name: "working", exact: true }).locator("svg"),
  ).toHaveCSS("opacity", "1");
  await expect(
    page.locator('.tree-session[data-state="stopped"] .session-indicator svg'),
  ).toHaveCount(0);
  await page.goto(
    "/iframe.html?id=sessions-panel--background-with-unread&viewMode=story",
  );
  await expect(
    page.getByRole("img", { name: "Unread response", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("img", { name: "Background tasks", exact: true }),
  ).toHaveCount(0);
});
