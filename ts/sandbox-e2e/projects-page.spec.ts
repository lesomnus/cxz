import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("projects use the full workspace with a persistent project picker and scoped sessions", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/projects");
  const main = page.getByRole("main", { name: "Project workspace" });
  await expect(main).toBeVisible({ timeout: 45000 });
  const picker = main.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(main.locator("header svg")).toBeVisible();
  await expect(main.locator("header svg")).toHaveCSS("width", "14px");
  await expect(
    page
      .getByRole("link", { name: "Projects view", exact: true })
      .locator("svg"),
  ).toHaveCSS("width", "20px");
  await expect(picker).toHaveText("Design playground");
  await page.mouse.move(0, 0);
  expect(
    await main.locator(".icon-value-menu").evaluate((el) => {
      const style = getComputedStyle(el, "::before");
      return { opacity: style.opacity, background: style.backgroundColor };
    }),
  ).toEqual({ opacity: "1", background: "rgb(32, 32, 32)" });
  await expect(page.getByRole("complementary")).toHaveCount(0);
  const sidebar = (await page
    .getByRole("navigation", { name: "Resources", exact: true })
    .boundingBox())!;
  expect((await main.boundingBox())!.x).toBe(sidebar.x + sidebar.width);
  const cards = main
    .getByRole("region", { name: "Project sessions" })
    .getByRole("button");
  await expect(cards).toHaveCount(8);
  await picker.click();
  const popup = page.getByRole("group", {
    name: "Select project choices",
    exact: true,
  });
  const motion = await popup.evaluate((el) => ({
    duration: 1000 * parseFloat(getComputedStyle(el).animationDuration),
    animations: el
      .getAnimations({ subtree: true })
      .filter((animation) => animation instanceof CSSAnimation)
      .map((animation) => animation.effect!.getTiming().duration),
  }));
  expect(motion.animations).toEqual([motion.duration, motion.duration]);
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/projects\?project=project-2$/);
  await expect(picker).toHaveText("Secondary project");
  await expect(cards).toHaveCount(1);
  await expect(cards).toContainText("session-8");
  await page.goBack();
  await expect(picker).toHaveText("Design playground");
  await page.goForward();
  await expect(picker).toHaveText("Secondary project");
  await page.reload();
  await expect(picker).toHaveText("Secondary project", { timeout: 45000 });
  await cards.focus();
  await cards.press("Enter");
  await expect(page).toHaveURL(/#\/projects\?project=project-2$/);
  await expect(cards).toHaveAttribute("aria-pressed", "true");
  await expect(
    main.getByRole("region", { name: "Session details", exact: true }),
  ).toContainText("codex");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveCount(0);
  await picker.click();
  await page
    .getByRole("option", { name: "Design playground", exact: true })
    .click();
  await expect(
    main.getByRole("region", { name: "Session details", exact: true }),
  ).toHaveCount(0);
});

test("project dropdown respects reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/sandbox.html#/projects");
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toBeEnabled({ timeout: 45000 });
  await picker.click();
  const popup = page.getByRole("group", {
    name: "Select project choices",
    exact: true,
  });
  await expect(popup).toBeVisible();
  expect(
    await popup.evaluate(
      (el) =>
        el
          .getAnimations({ subtree: true })
          .filter((animation) => animation instanceof CSSAnimation).length,
    ),
  ).toBe(0);
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await expect(picker).toHaveText("Secondary project");
});

test("project toolbar stays above the scrolling body on narrow screens", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 400 });
  await page.goto("/sandbox.html#/projects");
  const header = page.locator('main > header[aria-label="Project toolbar"]');
  await expect(header).toBeVisible({ timeout: 45000 });
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Design playground");
  const before = await header.boundingBox();
  const viewport = page.locator("main .panel-scroll-content");
  await viewport.evaluate((el) => (el.scrollTop = el.scrollHeight));
  expect(await viewport.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  expect(await header.boundingBox()).toEqual(before);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});
