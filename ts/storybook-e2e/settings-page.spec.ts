import { expect, test } from "@playwright/test";

const viewport = (page: import("@playwright/test").Page) =>
  page.locator("#settings-form .panel-scroll-content").first();
const topics = (page: import("@playwright/test").Page) =>
  page.getByRole("navigation", { name: "Settings topics" });

const koreanPack =
  /\/(?:src\/shared\/i18n\/locales\/ko\.ts|assets\/ko-[^/]+\.js)(?:\?|$)/;

test("language selection applies immediately after its lazy pack is ready", async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem(
      "settings",
      JSON.stringify({ "editor.tabSize": 8, future: { keep: true } }),
    ),
  );
  let releasePack!: () => void;
  const ready = new Promise<void>((resolve) => {
    releasePack = resolve;
  });
  let requests = 0;
  await page.route(koreanPack, async (route) => {
    requests++;
    await ready;
    await route.continue();
  });
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  const appearance = page.locator('.settings-group[aria-label="Appearance"]');
  await expect(appearance.locator(".setting-summary")).toHaveCount(0);
  await expect(appearance.locator(".setting-details")).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "Apply settings", exact: true }),
  ).toHaveCount(0);
  const language = page.getByRole("combobox", {
    name: "Display language",
    exact: true,
  });
  await language.click();
  await page.getByRole("option", { name: "한국어", exact: true }).click();
  await expect.poll(() => requests).toBe(1);
  await expect(language).toBeDisabled();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(
    await page.evaluate(
      () => JSON.parse(localStorage.getItem("settings")!)["ui.language"],
    ),
  ).toBeUndefined();
  releasePack();
  await expect(page.locator("html")).toHaveAttribute("lang", "ko");
  await expect(
    page.getByRole("heading", { name: "일반", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toEqual({
    "editor.tabSize": 8,
    future: { keep: true },
    "ui.language": "ko",
  });
  await page.getByRole("combobox", { name: "표시 언어", exact: true }).click();
  await page.getByRole("option", { name: "English", exact: true }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await language.click();
  await page.getByRole("option", { name: "한국어", exact: true }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "ko");
  expect(requests).toBe(1);
});

test("a failed language selection retains the active language and saved preferences", async ({
  page,
}) => {
  await page.route(koreanPack, (route) => route.abort("failed"));
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  const language = page.getByRole("combobox", {
    name: "Display language",
    exact: true,
  });
  await expect(language).toBeVisible();
  const before = await page.evaluate(() => localStorage.getItem("settings"));
  await language.click();
  await page.getByRole("option", { name: "한국어", exact: true }).click();
  await expect(page.locator(".settings-page [role=alert]")).toContainText(
    "Unable to apply the language",
  );
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(language).toBeEnabled();
  await expect(language).toHaveText("English");
  expect(await page.evaluate(() => localStorage.getItem("settings"))).toBe(
    before,
  );
});

test("settings panels are centered independently of the floating menu", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  const appearance = page.locator(".settings-group").filter({
    has: page.getByRole("heading", { name: "Appearance", exact: true }),
  });
  await expect(appearance).toHaveCount(1);
  await expect(appearance.locator(".setting-title")).toHaveText([
    "Language",
    "Theme",
  ]);
  await expect(appearance.locator(".setting-id")).toHaveText([
    "ui.language",
    "ui.theme",
  ]);
  const theme = appearance.getByRole("radiogroup", {
    name: "Theme",
    exact: true,
  });
  await expect(theme).toBeVisible();
  await expect(theme).toHaveText("");
  for (const name of ["Light", "Dark", "System"]) {
    await expect(theme.getByRole("radio", { name, exact: true })).toHaveCount(
      1,
    );
  }
  const profile = page.getByRole("button", { name: "Profile", exact: true });
  await profile.click();
  const profileMenu = page.getByRole("menu", { name: "Profile", exact: true });
  await expect(profileMenu).toBeVisible();
  const triggerBounds = (await profile.boundingBox())!;
  const menuBounds = (await profileMenu.boundingBox())!;
  expect(menuBounds.y + menuBounds.height).toBeCloseTo(
    triggerBounds.y + triggerBounds.height,
    0,
  );
  await page.keyboard.press("Escape");
  await expect(topics(page)).toBeVisible({ timeout: 20000 });
  await expect(page.locator("[data-settings-topic]")).toHaveCount(4);
  await expect(
    topics(page).getByRole("button", { name: "Auxiliary AI", exact: true }),
  ).toBeVisible();
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
  const table = page.getByRole("table", { name: "Connections", exact: true });
  await expect(
    table.getByRole("rowheader", { name: /This server/ }),
  ).toBeVisible();
  const insets = await table.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    const section = element.closest('[data-section-layout="section"]')!;
    const outer = section.getBoundingClientRect();
    return {
      left: bounds.left - outer.left,
      right: outer.right - bounds.right,
    };
  });
  expect(insets.left).toBeCloseTo(0, 1);
  expect(insets.right).toBeCloseTo(0, 1);
  const action = table.getByRole("button", {
    name: "Controls for This server",
  });
  const actionLayout = await action.evaluate((element) => {
    const row = element.closest("tr")!;
    const rowRect = row.getBoundingClientRect();
    const buttonRect = element.getBoundingClientRect();
    const tokens = getComputedStyle(row);
    const inset = parseFloat(tokens.getPropertyValue("--ui-control-inset"));
    return {
      size: buttonRect.width,
      height: buttonRect.height,
      end: rowRect.right - buttonRect.right,
      centerOffset:
        buttonRect.top +
        buttonRect.height / 2 -
        (rowRect.top + rowRect.height / 2),
      expectedSize:
        parseFloat(tokens.getPropertyValue("--resource-table-control-size")) -
        2 * inset,
      expectedEnd: 2 * inset,
    };
  });
  expect(actionLayout.size).toBeCloseTo(actionLayout.expectedSize, 1);
  expect(actionLayout.height).toBeCloseTo(actionLayout.size, 1);
  expect(actionLayout.end).toBeCloseTo(actionLayout.expectedEnd, 1);
  expect(actionLayout.centerOffset).toBeCloseTo(0, 1);
});

test("Logs remain a separate page accessible from Profile", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-settings--general&viewMode=story");
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Logs", exact: true }).click();
  await expect(
    page.getByRole("main", { name: "Logs", exact: true }),
  ).toBeVisible();
  await expect(topics(page)).toHaveCount(0);
});
