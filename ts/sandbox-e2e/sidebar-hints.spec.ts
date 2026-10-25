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

test("sidebar controls fit its inner width without horizontal scrolling at any viewport", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project");
  const sidebar = page.getByRole("navigation", {
    name: "Resources",
    exact: true,
  });
  await expect(
    sidebar.getByRole("link", { name: "Projects view", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  for (const viewport of [
    { width: 1440, height: 1000 },
    { width: 1280, height: 600 },
    { width: 390, height: 844 },
    { width: 390, height: 400 },
  ]) {
    await page.setViewportSize(viewport);
    const bounds = await sidebar.evaluate((element) => {
      const rect = element.getBoundingClientRect(),
        style = getComputedStyle(element);
      return {
        width: element.clientWidth,
        scrollWidth: element.scrollWidth,
        left: rect.left + parseFloat(style.paddingLeft),
        right:
          rect.right -
          parseFloat(style.borderRightWidth) -
          parseFloat(style.paddingRight),
        controls: [
          ...element.querySelectorAll("a, button, .profile-menu, .action-menu"),
        ].map((control) => {
          const bounds = control.getBoundingClientRect();
          return { left: bounds.left, right: bounds.right };
        }),
      };
    });
    expect(bounds.scrollWidth).toBe(bounds.width);
    for (const control of bounds.controls) {
      expect(control.left).toBeGreaterThanOrEqual(bounds.left);
      expect(control.right).toBeLessThanOrEqual(bounds.right);
    }
    expect(
      await sidebar.evaluate((element) => {
        element.scrollLeft = 100;
        return element.scrollLeft;
      }),
    ).toBe(0);
    await expect(sidebar).toHaveCSS("scrollbar-width", "none");
    const profile = sidebar.getByRole("button", {
      name: "Profile",
      exact: true,
    });
    await profile.scrollIntoViewIfNeeded();
    await expect(profile).toBeInViewport();
    await profile.click();
    await expect(
      page.getByRole("menu", { name: "Profile", exact: true }),
    ).toBeVisible();
    await page.keyboard.press("Escape");
  }
});
