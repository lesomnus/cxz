import { test, expect } from "@playwright/test";

for (const width of [1440, 390]) {
  test(`profile menu opens right of the sidebar and navigates at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/sandbox.html#/projects");
    const profile = page.getByRole("button", { name: "Profile", exact: true });
    await expect(profile).toBeVisible({ timeout: 45000 });
    const button = (await profile.boundingBox())!;
    expect(button.y).toBeGreaterThan(780);
    await profile.click();
    const menu = page.getByRole("menu", { name: "Profile", exact: true });
    await expect(menu).toBeVisible();
    const popup = (await menu.boundingBox())!;
    expect(popup.x).toBeGreaterThanOrEqual(button.x + button.width);
    expect(popup.x + popup.width).toBeLessThanOrEqual(width);
    expect(popup.y + popup.height).toBeLessThanOrEqual(900);
    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(profile).toBeFocused();
    await profile.press("Enter");
    await page.getByRole("menuitem", { name: "Settings", exact: true }).click();
    await expect(page).toHaveURL(/settings\/general/);
    await expect(menu).toBeHidden();
    await profile.click();
    await page.locator(".brand").click();
    await expect(menu).toBeHidden();
  });
}

test("profile exit invokes the sandbox logout callback", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/sandbox.html");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  const original = await input.elementHandle();
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Reset sandbox", exact: true })
    .click();
  await expect
    .poll(() => original!.evaluate((el) => el.isConnected))
    .toBe(false);
  await expect(input).toBeVisible();
});
