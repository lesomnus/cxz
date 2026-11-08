import { expect, test } from "@playwright/test";

const story = "/iframe.html?id=pages-sessions--default&viewMode=story";
const table = (page: import("@playwright/test").Page) =>
  page.getByRole("table", { name: "Sessions", exact: true });

for (const width of [390, 1280]) {
  test(`Sessions overview reuses table controls without a navigation panel at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(story);
    await expect(table(page)).toBeVisible();
    await expect(table(page).locator("tbody tr")).toHaveCount(10);
    expect(
      await table(page).getByRole("columnheader").allTextContents(),
    ).toEqual(["", "", "Name", "Account", "Project", ""]);
    await expect(
      page.getByRole("complementary", { name: "Session list", exact: true }),
    ).toHaveCount(0);
    await expect(page.locator(".session-panel-slot")).toHaveAttribute(
      "inert",
      "",
    );
    expect(
      await page
        .locator(".session-panel-slot")
        .evaluate((node) => node.getBoundingClientRect().width),
    ).toBe(0);
    await table(page).getByRole("checkbox").nth(1).check();
    await table(page).getByRole("checkbox").nth(2).check();
    await table(page)
      .getByRole("button", { name: "Selected session actions", exact: true })
      .click();
    await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
    const confirmation = page.getByRole("dialog", {
      name: "Stop sessions",
      exact: true,
    });
    await expect(confirmation).toBeVisible();
    await confirmation
      .getByRole("button", { name: "Cancel", exact: true })
      .click();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });
}

test("session navigation panel slides in with its reserved space and reverses on Back", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story);
  await expect(table(page)).toBeVisible();
  const slot = page.locator(".session-panel-slot");
  await page
    .locator(".workspace")
    .evaluate((node) =>
      (node as HTMLElement).style.setProperty(
        "--session-panel-motion-duration",
        "1500ms",
      ),
    );
  await table(page).locator("[data-session-navigation]").first().click();
  await expect(page.locator(".session-workspace")).toBeVisible();
  await expect(slot).not.toHaveAttribute("inert", "");
  await expect
    .poll(() => slot.evaluate((node) => node.getBoundingClientRect().width))
    .toBeGreaterThan(0);
  const entering = await slot.evaluate(
    (node) => node.getBoundingClientRect().width,
  );
  expect(entering).toBeLessThan(180);
  await expect
    .poll(() => slot.evaluate((node) => node.getBoundingClientRect().width))
    .toBe(180);
  await expect(
    page.getByRole("complementary", { name: "Session list", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Latest response", { exact: true }),
  ).toBeVisible();
  const conversation = await page.locator(".session-workspace").boundingBox();
  expect(conversation!.x).toBe(244);
  await page
    .getByRole("navigation", { name: "Preview history" })
    .getByRole("button", { name: "Back", exact: true })
    .click();
  await expect(slot).toHaveAttribute("inert", "");
  await expect(table(page)).toBeVisible();
  await expect
    .poll(() => slot.evaluate((node) => node.getBoundingClientRect().width))
    .toBe(0);
  expect(errors).toEqual([]);
});

test("narrow conversations hide navigation and browser Back restores the overview", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await page.goto(story);
  await expect(table(page)).toBeVisible();
  await table(page).locator("[data-session-navigation]").first().click();
  await expect(page.locator(".session-workspace")).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Session list", exact: true }),
  ).toHaveCount(0);
  const bounds = await page.locator(".session-workspace").boundingBox();
  expect(bounds!.x).toBe(0);
  expect(bounds!.width).toBe(390);
  await page
    .getByRole("navigation", { name: "Preview history" })
    .getByRole("button", { name: "Back", exact: true })
    .click();
  await expect(table(page)).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
});

for (const width of [390, 1280]) {
  test(`Sessions overview shares transparent table surfaces and sticks its header to page scrolling at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 600 });
    await page.goto("/iframe.html?id=pages-projects--list&viewMode=story");
    const projectTable = page.getByRole("table", {
      name: "Projects",
      exact: true,
    });
    await expect(projectTable).toBeVisible();
    const surface = await projectTable.evaluate((node) => ({
      table: getComputedStyle(node).backgroundColor,
      header: getComputedStyle(node.querySelector("thead")!).backgroundColor,
      blur: getComputedStyle(node.querySelector("thead")!).backdropFilter,
    }));

    await page.goto(
      "/iframe.html?id=pages-sessions--many-sessions&viewMode=story",
    );
    await expect(table(page).locator("tbody tr")).toHaveCount(50);
    await expect(table(page)).toHaveCSS("background-color", surface.table);
    const header = table(page).locator("thead");
    await expect(header).toHaveCSS("background-color", surface.header);
    await expect(header).toHaveCSS("backdrop-filter", surface.blur);
    await expect(header).toHaveCSS("position", "sticky");
    await expect(
      page
        .getByRole("region", { name: "Session list", exact: true })
        .locator(".panel-scroll-content"),
    ).toHaveCount(0);
    const viewport = page.locator(
      "[data-resource-page] > .panel-scroll-region > .panel-scroll-content",
    );
    await viewport.evaluate((node) => {
      node.scrollTop = 350;
    });
    await expect
      .poll(() => viewport.evaluate((node) => node.scrollTop))
      .toBe(350);
    expect(
      Math.abs(
        (await header.boundingBox())!.y - (await viewport.boundingBox())!.y,
      ),
    ).toBeLessThan(1);
    await table(page).locator("tbody tr").last().scrollIntoViewIfNeeded();
    await expect(table(page).locator("tbody tr").last()).toBeInViewport();
    expect(
      await page.evaluate(() => document.documentElement.scrollHeight),
    ).toBe(600);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
  });
}

test("reduced motion opens the panel without transitioning the workspace", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(story);
  await expect(table(page)).toBeVisible();
  await table(page).locator("[data-session-navigation]").first().click();
  await expect(page.locator(".workspace")).toHaveCSS(
    "transition-duration",
    "0s",
  );
  await expect(page.locator(".session-panel-slot > .session-panel")).toHaveCSS(
    "transition-duration",
    "0s",
  );
});
