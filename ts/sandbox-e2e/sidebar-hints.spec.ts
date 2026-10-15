import { expect, test, devices } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});
test("sidebar hints appear to the right without native titles and dismiss on navigation or Escape", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const sidebar = page.getByRole("navigation", {
    name: "Resources",
    exact: true,
  });
  const sessions = sidebar.getByRole("link", {
    name: "Sessions view",
    exact: true,
  });
  await expect(sessions).toBeVisible({ timeout: 45000 });
  await expect(sidebar.locator("a[title]")).toHaveCount(0);
  await sessions.hover();
  const tooltip = page.getByRole("tooltip", { name: "Sessions", exact: true });
  await expect(tooltip).toBeVisible();
  const anchorBox = (await sessions.boundingBox())!;
  const hintBox = (await tooltip.boundingBox())!;
  expect(hintBox.x).toBeGreaterThan(anchorBox.x + anchorBox.width);
  await page.keyboard.press("Escape");
  await expect(tooltip).toHaveCount(0);
  await sessions.focus();
  await expect(tooltip).toBeVisible();
  await expect(sessions).toHaveAttribute(
    "aria-describedby",
    (await tooltip.getAttribute("id")) as string,
  );
  await sessions.click();
  await expect(tooltip).toHaveCount(0);
});
