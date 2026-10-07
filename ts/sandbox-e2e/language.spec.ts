import { expect, test, devices, type Page } from "@playwright/test";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 2000, height: 1000 },
});
const packURL = /\/assets\/ko-[^/]+\.js/;
async function ready(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible({ timeout: 45000 });
}
async function language(page: Page) {
  if (
    !(await page
      .getByRole("button", { name: "Settings view", exact: true })
      .isVisible())
  )
    await page
      .getByRole("button", { name: "Back to sessions", exact: true })
      .click();
  await page
    .getByRole("button", { name: "Settings view", exact: true })
    .click();
  await page
    .getByRole("navigation", { name: "Settings topics", exact: true })
    .getByRole("button", { name: "Language", exact: true })
    .click();
}
test("English defaults and only Apply settings downloads the language pack, preserving JSON drafts and other preferences", async ({
  page,
}) => {
  await page.setViewportSize({ width: 2000, height: 1000 });
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.addInitScript(() =>
    localStorage.setItem(
      "settings",
      '{"editor.tabSize":8,"future":{"keep":true}}',
    ),
  );
  const requests: string[] = [];
  page.on("request", (request) => {
    if (packURL.test(request.url())) requests.push(request.url());
  });
  await ready(page);
  await language(page);
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  const editor = page.getByRole("textbox", {
    name: "settings.json",
    exact: true,
  });
  await expect(page.locator(".settings-file .monaco-editor")).toBeVisible({
    timeout: 30000,
  });
  const original = await editor.elementHandle();
  await page.evaluate(() =>
    navigator.clipboard.writeText('{"unsaved":"user text"}'),
  );
  await editor.press("Control+a");
  await editor.press("Control+v");
  await page.getByLabel("Display language", { exact: true }).selectOption("ko");
  await expect(
    page.getByRole("heading", { name: "Language", exact: true }),
  ).toBeVisible();
  expect(requests).toEqual([]);
  expect(
    await page.evaluate(
      () => JSON.parse(localStorage.getItem("settings")!)["ui.language"],
    ),
  ).toBeUndefined();
  await page
    .getByRole("button", { name: "Apply settings", exact: true })
    .click();
  await expect(page.locator("html")).toHaveAttribute("lang", "ko");
  expect(requests).toHaveLength(1);
  await expect(
    page.getByRole("heading", { name: "언어", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toEqual({
    "editor.tabSize": 8,
    future: { keep: true },
    "ui.language": "ko",
  });
  expect(await editor.evaluate((el, old) => el === old, original)).toBe(true);
  await expect(page.locator(".settings-file .view-lines")).toContainText(
    "unsaved",
  );
  await editor.press("Control+z");
  await expect(page.locator(".settings-file .view-lines")).not.toContainText(
    "unsaved",
  );
  await expect(page.locator(".settings-file .view-lines")).toContainText(
    "editor.tabSize",
  );
  await page
    .getByRole("navigation", { name: "설정 주제", exact: true })
    .getByRole("button", { name: "에디터", exact: true })
    .click();
  await expect(
    page.getByLabel("전역 에디터 Tab 문자 표시 폭", { exact: true }),
  ).toHaveValue("8");
  await page.screenshot({ path: "test-results/settings-language-ko.png" });
});
test("failed language download leaves the active language and saved file unchanged", async ({
  page,
}) => {
  await page.route(packURL, (route) => route.abort("failed"));
  await ready(page);
  await language(page);
  const before = await page.evaluate(() => localStorage.getItem("settings"));
  await page.getByLabel("Display language", { exact: true }).selectOption("ko");
  await page
    .getByRole("button", { name: "Apply settings", exact: true })
    .click();
  await expect(
    page.locator(".settings-page p[role=alert]:visible"),
  ).toContainText("Unable to apply the language");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(await page.evaluate(() => localStorage.getItem("settings"))).toBe(
    before,
  );
  await expect(
    page.getByRole("button", { name: "Apply settings", exact: true }),
  ).toBeEnabled();
});
test("saved language loads on startup and cross-tab language changes preserve the composer and its Undo", async ({
  page,
}) => {
  await page.addInitScript(() => {
    if (!localStorage.getItem("settings"))
      localStorage.setItem("settings", '{"ui.language":"ko"}');
  });
  await page.goto("/sandbox.html");
  await expect(page.locator("html")).toHaveAttribute("lang", "ko");
  const input = page.getByRole("textbox", { name: "메시지", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  const original = await input.elementHandle();
  const viewer = page.getByRole("complementary", {
    name: "워크스페이스 에디터",
    exact: true,
  });
  await viewer.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(viewer.locator(".monaco-editor")).toBeVisible({
    timeout: 30000,
  });
  const fileInput = await viewer
    .locator(".monaco-editor textarea")
    .elementHandle();
  await input.fill("user text 한글");
  await input.press("End");
  await input.press("Tab");
  await expect(input).toHaveValue("user text 한글  ");
  await page.evaluate(() => {
    const raw = '{"ui.language":"en"}';
    localStorage.setItem("settings", raw);
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "settings",
        newValue: raw,
        storageArea: localStorage,
      }),
    );
  });
  const english = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  expect(await english.evaluate((el, old) => el === old, original)).toBe(true);
  const englishViewer = page.getByRole("complementary", {
    name: "Workspace editor",
    exact: true,
  });
  expect(
    await englishViewer
      .locator(".monaco-editor textarea")
      .evaluate((el, old) => el === old, fileInput),
  ).toBe(true);
  await expect(
    englishViewer.locator(".monaco-editor textarea"),
  ).toHaveJSProperty("readOnly", true);
  await expect(english).toHaveValue("user text 한글  ");
  await english.press("Control+z");
  await expect(english).toHaveValue("user text 한글");
  await page.reload();
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
});
