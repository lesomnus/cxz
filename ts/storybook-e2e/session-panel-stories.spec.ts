import { expect, test } from "@playwright/test";

for (const [story, rows] of [
  ["default", 3],
  ["long-list", 24],
  ["collapsed", 0],
  ["empty", 0],
  ["loading", 0],
  ["unread-completed", 3],
  ["error-before-completion", 4],
  ["background-only", 3],
  ["background-with-unread", 3],
  ["lifecycle-states", 5],
  ["unread-beyond-scroll", 24],
] as const) {
  test(`session panel story ${story} renders`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(`/iframe.html?id=sessions-panel--${story}&viewMode=story`);
    await expect(page.locator(".tree-project")).toBeVisible();
    await expect(page.locator(".tree-session")).toHaveCount(rows);
    if (rows) {
      await page.locator(".tree-project").click();
      await expect(page.locator(".tree-session")).toHaveCount(0);
      await page.locator(".tree-project").click();
      await expect(page.locator(".tree-session")).toHaveCount(rows);
    }
    expect(errors).toEqual([]);
  });
}
