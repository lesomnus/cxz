import { expect, test } from "@playwright/test";

for (const width of [390, 1280]) {
  test(`settings use text navigation and resource sections at ${width}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
    const topics = page.getByRole("navigation", { name: "Settings topics" });
    await expect(topics).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Display language", exact: true }),
    ).toBeVisible();
    await page.screenshot({ path: info.outputPath("general.png") });
    await expect(
      page.getByRole("complementary", { name: "Settings navigation" }),
    ).toHaveCount(0);
    await expect(
      topics.getByRole("link", { name: "General", exact: true }),
    ).toHaveAttribute("aria-current", "page");
    await expect(
      topics.getByRole("link", { name: "Logs", exact: true }),
    ).toHaveCount(0);
    const editor = topics.getByRole("link", { name: "Editor", exact: true });
    await editor.hover();
    expect(
      await editor.evaluate((node) => getComputedStyle(node).backgroundColor),
    ).toBe("rgba(0, 0, 0, 0)");
    await editor.click();
    await expect(
      page.getByRole("region", { name: "Editor settings" }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Global editor", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Session editor", exact: true }),
    ).toHaveCount(1);
    await expect(page.locator(".settings-group").first()).toHaveAttribute(
      "data-section-layout",
      "section",
    );
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
test("settings.json can replace the form on narrow screens and docks on wide screens", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/iframe.html?id=pages-settings--editor&viewMode=story");
  const toggle = page.getByRole("button", {
    name: "Edit settings.json",
    exact: true,
  });
  await toggle.click();
  await expect(
    page.getByRole("complementary", { name: "Settings file editor" }),
  ).toBeVisible();
  await expect(
    page.locator("#settings-file-editor .monaco-editor"),
  ).toBeVisible();
  await expect(page.locator("#settings-editor")).toBeHidden();
  await toggle.click();
  await expect(page.locator("#settings-editor")).toBeVisible();
  await page.setViewportSize({ width: 2100, height: 1000 });
  await expect(page.locator("#settings-editor")).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Settings file editor" }),
  ).toBeVisible();
  await expect(
    page.locator("#settings-file-editor .monaco-editor"),
  ).toBeVisible();
});

test("settings controls still save and Connections shares the text navigation", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  const sound = page.getByRole("switch", {
    name: "Notification sounds",
    exact: true,
  });
  await expect(sound).toBeChecked({ timeout: 20000 });
  const before = await page.locator(".settings-editor-groups").boundingBox();
  await sound.click();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          JSON.parse(localStorage.getItem("settings")!)["notifications.sound"],
      ),
    )
    .toBe(false);
  await expect(
    page.locator("main.settings-page").getByRole("status"),
  ).toContainText("Saved.");
  expect((await page.locator(".settings-editor-groups").boundingBox())?.y).toBe(
    before?.y,
  );
  const topics = page.getByRole("navigation", { name: "Settings topics" });
  await topics.getByRole("link", { name: "Connections", exact: true }).click();
  await expect(
    topics.getByRole("link", { name: "Connections", exact: true }),
  ).toHaveAttribute("aria-current", "page");
  await expect(
    page.getByRole("cell", { name: "https://cxz.example", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 900 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
  ).toBe(false);
});
test("Logs are reached from the sidebar without settings navigation", async ({
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
  await expect(
    page.getByRole("navigation", { name: "Settings topics" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Logs", exact: true }),
  ).toHaveAttribute("aria-current", "page");
});
