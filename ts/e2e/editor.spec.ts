import { test, expect, devices } from "@playwright/test";

// Playwright runs in Node; the shared browser tsconfig has no Node globals.
declare const process: { env: Record<string, string | undefined> };

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 2000, height: 1000 },
});

test("production file viewer loads under the gateway CSP", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.getByRole("link", { name: /demo-chat/ }).click();
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await expect(editor).toBeVisible();
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    "Fixture workspace",
    { timeout: 30000 },
  );
  await expect(editor.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await expect(editor.locator(".monaco-editor")).toHaveCSS(
    "background-color",
    "rgb(20, 20, 20)",
  );
  await expect(page.locator(".markdown [style]")).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("editable settings JSON and its worker load under production CSP and save the single settings file", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  const pane = page.getByRole("complementary", {
    name: "Settings file editor",
    exact: true,
  });
  const input = pane.getByRole("textbox", {
    name: "settings.json",
    exact: true,
  });
  await expect(pane.locator(".monaco-editor")).toBeVisible({ timeout: 30000 });
  await expect(input).toHaveJSProperty("readOnly", false);
  await expect(pane.locator(".monaco-editor")).toHaveCSS(
    "background-color",
    "rgb(20, 20, 20)",
  );
  await page
    .getByRole("navigation", { name: "Settings topics" })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
  const raw = '{\n  "editor.tabSize": 6,\n  "editor.colorPalette": "cool"\n}\n';
  await page.evaluate((raw) => navigator.clipboard.writeText(raw), raw);
  await input.press("Control+a");
  await input.press("Control+v");
  await expect(pane.locator(".view-lines")).toContainText(
    '"editor.tabSize": 6',
  );
  await input.press("Control+Enter");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("settings")))
    .toBe(raw);
  await expect(
    page.getByLabel("Global editor Tab display width", { exact: true }),
  ).toHaveValue("6");
  await expect(
    pane.locator(".view-lines").getByText('"editor.tabSize"', { exact: true }),
  ).toHaveCSS("color", "rgb(137, 155, 170)");
  // Wait for real JSON diagnostics to ensure the dedicated worker is covered.
  await input.press("Control+End");
  await input.press("x");
  await expect(pane.locator(".squiggly-error").first()).toBeVisible({
    timeout: 10000,
  });
  await input.press("Control+z");
  expect(errors).toEqual([]);
});

test("real container editor opens the workspace through the authenticated iframe", async ({
  page,
}) => {
  test.setTimeout(90000);
  test.skip(
    !process.env.CXZ_EDITOR_PROBE_CONTAINER,
    "opt-in real OpenVSCode/container probe",
  );
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  await page.getByRole("link", { name: /demo-chat/ }).click();
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await editor.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText(
    "Fixture workspace",
  );
  await editor.getByRole("button", { name: "Connect", exact: true }).click();
  await expect(
    editor.getByRole("button", { name: "Disconnect", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  const frame = page.frameLocator('iframe[title="VS Code workspace"]');
  await expect(frame.locator(".monaco-workbench")).toBeVisible({
    timeout: 45000,
  });
  const trust = frame.getByRole("button", { name: /Yes, I trust/ });
  // The workbench becomes visible before its first-use trust dialog appears.
  await trust.click({ timeout: 15000 });
  await expect(
    frame.getByText("README.md", { exact: true }).first(),
  ).toBeVisible({ timeout: 30000 });
  await frame.getByText("README.md", { exact: true }).first().dblclick();
  await expect(frame.locator(".editor-instance .view-lines")).toContainText(
    "Editor probe",
    {
      timeout: 30000,
    },
  );
  await expect(editor.locator("iframe")).toHaveCSS("filter", "none");
  await page.screenshot({
    path: "test-results/real-workspace-editor.png",
    fullPage: true,
  });
  await editor.getByRole("button", { name: "Disconnect", exact: true }).click();
  await expect(editor.locator("iframe")).toHaveCount(0);
  await expect(
    editor.getByRole("button", { name: "README.md", exact: true }),
  ).toBeVisible();
});

test("language packs load only on Apply under the production CSP and the saved language survives reload", async ({
  page,
}) => {
  const requests: string[] = [];
  const errors: string[] = [];
  page.on("request", (request) => {
    if (/\/assets\/ko-[^/]+\.js/.test(request.url()))
      requests.push(request.url());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await page.getByLabel("Web access token").fill("a".repeat(32));
  await page.getByRole("button", { name: "Connect", exact: true }).click();
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Settings topics", exact: true })
    .getByRole("link", { name: "General", exact: true })
    .click();
  await page.getByLabel("Display language", { exact: true }).click();
  await page.getByRole("option", { name: "한국어", exact: true }).click();
  expect(requests).toEqual([]);
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await page
    .getByRole("button", { name: "Apply settings", exact: true })
    .click();
  await expect(page.locator("html")).toHaveAttribute("lang", "ko");
  expect(requests).toHaveLength(1);
  expect(
    await page.evaluate(
      () => JSON.parse(localStorage.getItem("settings")!)["ui.language"],
    ),
  ).toBe("ko");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "ko");
  await expect(
    page.getByRole("link", { name: "설정 보기", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
