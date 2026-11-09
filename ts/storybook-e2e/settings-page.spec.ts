import { expect, test } from "@playwright/test";

const viewport = (page: import("@playwright/test").Page) =>
  page.locator("#settings-form .panel-scroll-content").first();
const topics = (page: import("@playwright/test").Page) =>
  page.getByRole("navigation", { name: "Settings topics" });

test("settings panels are centered independently of the floating menu", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  await expect(topics(page)).toBeVisible({ timeout: 20000 });
  await expect(page.locator("[data-settings-topic]")).toHaveCount(3);
  const bounds = await page
    .locator("#settings-form [data-resource-layout]")
    .boundingBox();
  const scroll = await viewport(page).boundingBox();
  const menu = await topics(page).boundingBox();
  expect(
    Math.abs(bounds!.x + bounds!.width / 2 - (scroll!.x + scroll!.width / 2)),
  ).toBeLessThan(1);
  expect(menu!.x + menu!.width).toBeLessThan(bounds!.x);
  const general = topics(page).getByRole("button", {
    name: "General",
    exact: true,
  });
  const editor = topics(page).getByRole("button", {
    name: "Editor",
    exact: true,
  });
  await expect(general).toHaveAttribute("aria-current", "location");
  await page.screenshot({ path: info.outputPath("general-centered.png") });
  await editor.hover();
  expect(
    await editor.evaluate((node) => getComputedStyle(node).backgroundColor),
  ).toBe("rgba(0, 0, 0, 0)");
  await editor.click();
  await expect(editor).toHaveAttribute("aria-current", "location");
  await expect(page.locator("[data-settings-topic=general]")).toHaveCount(1);
  await viewport(page).evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(
    topics(page).getByRole("button", { name: "Connections", exact: true }),
  ).toHaveAttribute("aria-current", "location");
  await viewport(page).evaluate((node) => {
    node.scrollTop = 0;
  });
  await expect(general).toHaveAttribute("aria-current", "location");
});

for (const width of [390, 1024]) {
  test(`an obscured menu becomes a toolbar dropdown at ${width}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
    const picker = page.getByRole("combobox", { name: "Settings topics" });
    await expect(picker).toBeVisible({ timeout: 20000 });
    await expect(topics(page)).toHaveCount(0);
    await picker.click();
    await page.getByRole("option", { name: "Editor", exact: true }).click();
    await expect(picker).toHaveText("Editor");
    await viewport(page).evaluate((node) => {
      node.scrollTop = 0;
    });
    await expect(picker).toHaveText("General");
    await page.screenshot({ path: info.outputPath("compact-menu.png") });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
    ).toBe(false);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollHeight > innerHeight,
      ),
    ).toBe(false);
  });
}

test("the floating menu and dropdown switch when their available space changes", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  await expect(topics(page)).toBeVisible({ timeout: 20000 });
  await page.setViewportSize({ width: 1024, height: 900 });
  await expect(
    page.getByRole("combobox", { name: "Settings topics" }),
  ).toBeVisible();
  await page.setViewportSize({ width: 1280, height: 900 });
  await expect(topics(page)).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Settings topics" }),
  ).toHaveCount(0);
});

test("settings.json can replace the form and categories reveal it again", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=pages-settings--editor&viewMode=story");
  const toggle = page.getByRole("button", {
    name: "Edit settings.json",
    exact: true,
  });
  await toggle.click();
  await expect(
    page.locator("#settings-file-editor .monaco-editor"),
  ).toBeVisible();
  await expect(page.locator("#settings-form")).toBeHidden();
  const picker = page.getByRole("combobox", { name: "Settings topics" });
  await picker.click();
  await page.getByRole("option", { name: "General", exact: true }).click();
  await expect(page.locator("#settings-form")).toBeVisible();
  await page.setViewportSize({ width: 2100, height: 1000 });
  await expect(
    page.locator("#settings-file-editor .monaco-editor"),
  ).toBeVisible();
  await expect(page.locator("#settings-form")).toBeVisible();
});

test("all categories coexist, save without moving panels, and retain deep links", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  const sound = page.getByRole("switch", {
    name: "Notification sounds",
    exact: true,
  });
  await expect(sound).toBeChecked({ timeout: 20000 });
  const before = await page
    .locator(".settings-editor-groups")
    .first()
    .boundingBox();
  await sound.click();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          JSON.parse(localStorage.getItem("settings")!)["notifications.sound"],
      ),
    )
    .toBe(false);
  expect(
    (await page.locator(".settings-editor-groups").first().boundingBox())?.y,
  ).toBe(before?.y);
  await expect(
    page.getByRole("heading", { name: "Global editor", exact: true }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("cell", { name: "https://cxz.example", exact: true }),
  ).toHaveCount(1);
  await page.goto("/iframe.html?id=pages-settings--connections&viewMode=story");
  await expect(
    topics(page).getByRole("button", { name: "Connections", exact: true }),
  ).toHaveAttribute("aria-current", "location");
});

test("Logs remain a separate page accessible from the sidebar", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  await page
    .getByRole("navigation", { name: "Resources", exact: true })
    .getByRole("link", { name: "Logs", exact: true })
    .click();
  await expect(
    page.getByRole("main", { name: "Logs", exact: true }),
  ).toBeVisible();
  await expect(topics(page)).toHaveCount(0);
});
