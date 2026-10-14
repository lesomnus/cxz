import { expect, test } from "@playwright/test";

test("unread errors override completed session indicators", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=sessions-panel--error-before-completion&viewMode=story",
  );
  const error = page.getByRole("img", { name: "Unread error", exact: true });
  await expect(error).toBeVisible();
  await expect(error).toHaveCSS("color", "rgb(255, 82, 109)");
  await expect(
    page.getByRole("img", { name: "Unread response", exact: true }),
  ).toBeVisible();
});
test("navigation badge uses the same error color", async ({ page }) => {
  await page.goto(
    "/iframe.html?id=navigation-resourceicon--unread-error&viewMode=story",
  );
  const badge = page.getByRole("img", { name: "Unread error", exact: true });
  await expect(badge).toBeVisible();
  await expect(badge).toContainText("3");
  await expect(badge).toHaveCSS("background-color", "rgb(255, 82, 109)");
});
