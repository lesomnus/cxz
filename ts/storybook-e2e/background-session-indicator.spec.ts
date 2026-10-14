import { expect, test } from "@playwright/test";
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
