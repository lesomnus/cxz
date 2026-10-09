import { chooseSetting, expectInherited } from "./settings-controls";
import {
  expect,
  test,
  devices,
  type Page,
  type Locator,
} from "@playwright/test";

test.use({
  userAgent: devices["Desktop Chrome"].userAgent,
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
  deviceScaleFactor: 1,
});

async function ready(page: Page) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
async function settings(page: Page) {
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Settings topics" })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Editor", exact: true }),
  ).toBeVisible();
}
async function file(page: Page) {
  const input = page.getByRole("textbox", {
    name: "settings.json",
    exact: true,
  });
  if (
    !(await page
      .getByRole("complementary", { name: "Settings file editor", exact: true })
      .isVisible())
  )
    await page.getByRole("button", { name: /^Edit settings\.json/ }).click();
  await expect(input).toBeAttached({ timeout: 30000 });
  return input;
}
// Exercise the real editor through keyboard copy/paste, without a production test API.
async function writeJSON(page: Page, input: Locator, value: string) {
  await page.evaluate((value) => navigator.clipboard.writeText(value), value);
  await input.press("Control+a");
  await input.press("Control+v");
  // Monaco applies clipboard input asynchronously. Wait for the edited text
  // before a subsequent Save command can read the previous model contents.
  await expect
    .poll(async () =>
      (await page.locator(".settings-file .view-lines").textContent())?.replace(
        /\s/g,
        "",
      ),
    )
    .toBe(value.replace(/\s/g, ""));
}
async function readJSON(page: Page, input: Locator) {
  await input.press("Control+a");
  await input.press("Control+c");
  const value = await page.evaluate(() => navigator.clipboard.readText());
  await input.press("Control+u"); // Restore the cursor/selection after Select All.
  // Monaco uses platform line endings for clipboard text; stored/exported bytes
  // are checked separately against the original JSON document.
  return value.replaceAll("\r\n", "\n");
}
async function stored(page: Page) {
  return page.evaluate(() => JSON.parse(localStorage.getItem("settings")!));
}
const fields = [
  "Indentation size",
  "Tab input",
  "Tab display width",
  "Color palette",
];

test("settings topics keep compact columns centered and a live JSON pane unfolds at the conversation width threshold", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1843, height: 1000 });
  await ready(page);
  await settings(page);
  const topics = page.getByRole("navigation", {
    name: "Settings topics",
    exact: true,
  });
  const editorTopic = topics.getByRole("link", {
    name: "Editor",
    exact: true,
  });
  await expect(editorTopic).toHaveAttribute("aria-current", "page");
  await expect(topics.getByRole("link")).toHaveCount(2);
  await expect(page.getByRole("tablist")).toHaveCount(0);
  const body = page.locator(".settings-editor-body");
  const form = page.getByRole("region", {
    name: "Editor settings",
    exact: true,
  });
  const pane = page.getByRole("complementary", {
    name: "Settings file editor",
    exact: true,
  });
  const source = page.getByRole("textbox", {
    name: "settings.json",
    exact: true,
  });
  const centered = () =>
    body.evaluate((el) => {
      const body = el.getBoundingClientRect();
      const pane = el.parentElement!.getBoundingClientRect();
      return {
        width: body.width,
        limit: parseFloat(getComputedStyle(el).maxWidth),
        delta: Math.abs(
          body.left + body.width / 2 - (pane.left + pane.width / 2),
        ),
      };
    });
  const expectCentered = async () => {
    const current = await centered();
    expect(current.delta).toBeLessThan(0.1);
    expect(current.width).toBe(current.limit);
  };
  await expectCentered();
  await expect(pane).toBeHidden();
  await expect(page.locator(".settings-file .monaco-editor")).toHaveCount(0);
  await page.setViewportSize({ width: 1844, height: 1000 });
  await expect(pane).toBeVisible();
  expect((await form.boundingBox())!.width).toBe(800);
  expect((await pane.boundingBox())!.width).toBe(800);
  await expect(pane).toHaveCSS("border-left-width", "1px");
  await expectCentered();
  await chooseSetting(page, "Global editor Tab display width", "8");
  const saved = '{\n  "editor.tabSize": 8\n}\n';
  await expect.poll(() => readJSON(page, source)).toBe(saved);
  await source.press("Control+End");
  await source.press("Enter");
  await source.evaluate((el) => {
    (el as HTMLElement).dataset.identity = "original";
  });
  await page.setViewportSize({ width: 1843, height: 1000 });
  await expect(source).toBeVisible();
  await expect(form).toBeHidden();
  await expect(source).toBeFocused();
  await expect.poll(() => readJSON(page, source)).toBe(saved + "\n");
  await editorTopic.click();
  await expect(form).toBeVisible();
  await expect(source).toBeHidden();
  await expectCentered();
  await page.setViewportSize({ width: 2104, height: 1000 });
  await expect(source).toBeVisible();
  await expect(source).toHaveAttribute("data-identity", "original");
  await expect.poll(() => readJSON(page, source)).toBe(saved + "\n");
  await source.press("Control+z");
  await expect.poll(() => readJSON(page, source)).toBe(saved);
  expect(await page.evaluate(() => localStorage.getItem("settings"))).toBe(
    saved,
  );
  await expectCentered();
  await page.screenshot({
    path: "test-results/settings-wide.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(source).toBeVisible();
  await expect(topics).toBeVisible();
  await page
    .getByRole("button", { name: "Back to settings", exact: true })
    .click();
  await expect(form).toBeVisible();
  await expect(source).toBeHidden();
  expect((await centered()).width).toBeLessThanOrEqual(600);
  expect((await centered()).delta).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({
    path: "test-results/settings-topics-mobile.png",
    fullPage: true,
  });
});

test("settings JSON and session files share the editor surface and theme while only JSON is editable", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.setViewportSize({ width: 2000, height: 1000 });
  await ready(page);
  const workspace = page.getByRole("complementary", {
    name: "Workspace editor",
    exact: true,
  });
  await workspace.getByRole("button", { name: "src", exact: true }).click();
  await workspace.getByRole("button", { name: "main.go", exact: true }).click();
  await expect(workspace.locator(".view-lines")).toContainText("package main", {
    timeout: 30000,
  });
  const appearance = (host: Locator) =>
    host.evaluate((el) => {
      const editor = getComputedStyle(el.querySelector(".monaco-editor")!);
      const gutter = getComputedStyle(el.querySelector(".margin")!);
      const lines = getComputedStyle(el.querySelector(".view-lines")!);
      return {
        background: editor.backgroundColor,
        gutter: gutter.backgroundColor,
        fontSize: lines.fontSize,
        fontFamily: lines.fontFamily,
        lineHeight: lines.lineHeight,
      };
    });
  const expected = await appearance(workspace);
  expect(expected.background).toBe("rgb(20, 20, 20)");
  expect(expected.fontSize).toBe("12px");
  const headerPadding = await workspace
    .locator("header")
    .evaluate((el) => getComputedStyle(el).padding);
  const footerFont = await workspace
    .locator("footer")
    .evaluate((el) => getComputedStyle(el).fontSize);
  await settings(page);
  const pane = page.getByRole("complementary", {
    name: "Settings file editor",
    exact: true,
  });
  const input = await file(page);
  await expect(pane.locator(".monaco-editor")).toBeVisible();
  expect(await appearance(pane)).toEqual(expected);
  expect(
    await pane.locator("header").evaluate((el) => getComputedStyle(el).padding),
  ).toBe(headerPadding);
  expect(
    await pane
      .locator("footer")
      .evaluate((el) => getComputedStyle(el).fontSize),
  ).toBe(footerFont);
  await expect(input).toHaveJSProperty("readOnly", false);
  await expect(
    pane.locator(".margin-view-overlays .line-numbers").first(),
  ).toHaveText("1");
  await writeJSON(
    page,
    input,
    '{\n  "editor.tabSize": 8,\n  "editor.colorPalette": "cool"\n}\n',
  );
  await expect(
    pane.getByRole("button", { name: "Save", exact: true }),
  ).toBeEnabled();
  await input.press("Control+Enter");
  await expect
    .poll(() => stored(page))
    .toEqual({
      "editor.tabSize": 8,
      "editor.colorPalette": "cool",
    });
  await expect(
    pane.locator(".view-lines").getByText('"editor.tabSize"', { exact: true }),
  ).toHaveCSS("color", "rgb(137, 155, 170)");
  await page.screenshot({
    path: "test-results/settings-shared-editor.png",
    fullPage: true,
  });
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(
    workspace.getByRole("tab", { name: "main.go", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(workspace.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await expect(
    workspace
      .locator(".view-line")
      .first()
      .locator("span")
      .filter({ hasText: "package" })
      .last(),
  ).toHaveCSS("color", "rgb(126, 143, 175)");
  expect(await appearance(workspace)).toEqual(expected);
  expect(errors).toEqual([]);
});

test("one settings file persists, each session field overrides or inherits and edits keep the conversation draft", async ({
  page,
}) => {
  await ready(page);
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  await composer.fill("keep draft");
  await settings(page);
  for (const label of fields)
    await expectInherited(page, `Session editor ${label}`);
  for (const [label, value] of fields.map((label, index) => [
    label,
    ["4", "false", "8", "cool"][index],
  ]))
    await chooseSetting(page, `Global editor ${label}`, value);
  for (const [label, value] of fields.map((label, index) => [
    label,
    ["6", "true", "2", "monochrome"][index],
  ]))
    await chooseSetting(page, `Session editor ${label}`, value);
  expect(await stored(page)).toEqual({
    "editor.indentSize": 4,
    "editor.insertSpaces": false,
    "editor.tabSize": 8,
    "editor.colorPalette": "cool",
    "session.editor.indentSize": 6,
    "session.editor.insertSpaces": true,
    "session.editor.tabSize": 2,
    "session.editor.colorPalette": "monochrome",
  });
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(composer).toHaveValue("keep draft");
  await composer.press("End");
  await composer.press("Tab");
  await expect(composer).toHaveValue("keep draft      ");
  await expect(composer).toHaveCSS("tab-size", "2");
  await composer.fill("```javascript\nconst n = 42;\n```");
  await expect(page.locator(".editor-code-line .hljs-keyword")).toHaveCSS(
    "color",
    "rgb(237, 237, 237)",
  );
  await settings(page);
  for (const label of fields)
    await chooseSetting(page, `Session editor ${label}`, "");
  expect(await stored(page)).toEqual({
    "editor.indentSize": 4,
    "editor.insertSpaces": false,
    "editor.tabSize": 8,
    "editor.colorPalette": "cool",
  });
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  await expect(composer).toHaveCSS("tab-size", "8");
  await expect(page.locator(".editor-code-line .hljs-keyword")).toHaveCSS(
    "color",
    "rgb(126, 143, 175)",
  );
  await composer.fill("value");
  await composer.press("Home");
  await composer.press("Tab");
  await expect(composer).toHaveValue("\tvalue");
  await composer.press("Shift+Tab");
  await expect(composer).toHaveValue("value");
  await page.reload();
  await expect(composer).toBeVisible({ timeout: 45000 });
  await expect(composer).toHaveCSS("tab-size", "8");
  await composer.press("Tab");
  await expect(composer).toHaveValue("\t");
  await chooseSetting(page, "Scenario", "session-4");
  const other = page.getByRole("textbox", {
    name: "Other answer: Which environment?",
    exact: true,
  });
  await expect(other).toHaveCSS("tab-size", "8");
  await other.fill("answer");
  await other.press("Home");
  await other.press("Tab");
  await expect(other).toHaveValue("\tanswer");
  await page.screenshot({
    path: "test-results/settings-composer.png",
    fullPage: true,
  });
});

test("JSON editing, validation, export and import preserve unknown settings in the same file", async ({
  page,
}) => {
  await ready(page);
  await settings(page);
  const source = await file(page);
  const raw =
    '{\n  "editor.tabSize": 8,\n  "future": {"enabled": true},\n  "session.editor.insertSpaces": false\n}\n';
  await writeJSON(page, source, raw);
  await source.press("Control+Enter");
  await expect(
    page.locator(".settings-page p[role=status]:visible"),
  ).toContainText("Save");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("settings")))
    .toBe(raw);
  await source.press("Control+Home");
  await source.press("Tab");
  await expect.poll(() => readJSON(page, source)).toBe("  " + raw);
  await source.press("Control+z");
  await expect.poll(() => readJSON(page, source)).toBe(raw);
  await source.press("Control+m");
  await source.press("Tab");
  await expect(
    page.getByRole("button", { name: "Reload saved file", exact: true }),
  ).toBeFocused();
  await page
    .getByRole("navigation", { name: "Settings topics", exact: true })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
  await expect(
    page.getByLabel("Global editor Tab display width", { exact: true }),
  ).toHaveValue("8");
  await chooseSetting(page, "Global editor Color palette", "warm");
  expect((await stored(page)).future).toEqual({ enabled: true });
  await file(page);
  const saved = await readJSON(page, source);
  await writeJSON(page, source, '{"editor.tabSize":0}');
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    page.locator(".settings-page p[role=alert]:visible"),
  ).toContainText("between 1 and 16");
  expect(await page.evaluate(() => localStorage.getItem("settings"))).toBe(
    saved,
  );
  await page
    .getByRole("button", { name: "Reload saved file", exact: true })
    .click();
  await expect.poll(() => readJSON(page, source)).toBe(saved);
  const downloadPending = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export", exact: true }).click();
  const download = await downloadPending;
  expect(download.suggestedFilename()).toBe("settings.json");
  const stream = await download.createReadStream();
  let exported = "";
  await new Promise<void>((resolve, reject) => {
    stream!.on("data", (chunk: unknown) => {
      exported += String(chunk);
    });
    stream!.on("end", resolve);
    stream!.on("error", reject);
  });
  expect(exported).toBe(saved);
  const imported =
    '{"editor.indentSize":3,"session.editor.tabSize":6,"future":[1,2]}';
  await page
    .getByLabel("settings.json Import", { exact: true })
    .evaluate((el: HTMLInputElement, raw) => {
      const files = new DataTransfer();
      files.items.add(
        new File([raw], "settings.json", { type: "application/json" }),
      );
      el.files = files.files;
      el.dispatchEvent(new Event("change", { bubbles: true }));
    }, imported);
  await expect.poll(() => readJSON(page, source)).toBe(imported);
  expect(await page.evaluate(() => localStorage.getItem("settings"))).toBe(
    imported,
  );
  await page
    .getByRole("navigation", { name: "Settings topics", exact: true })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
  await expect(
    page.getByLabel("Session editor Tab display width", { exact: true }),
  ).toHaveValue("6");
  await page.screenshot({
    path: "test-results/settings-editor.png",
    fullPage: true,
  });
});

test("cross-tab changes refresh editors and preserve stale JSON drafts instead of overwriting newer settings", async ({
  page,
  context,
}) => {
  await ready(page);
  await settings(page);
  const source = await file(page);
  await writeJSON(page, source, '{"editor.tabSize":2}');
  const other = await context.newPage();
  await ready(other);
  await settings(other);
  await chooseSetting(other, "Global editor Tab display width", "8");
  await expect(
    page.locator(".settings-page p[role=alert]:visible"),
  ).toContainText("Your draft was preserved");
  await expect.poll(() => readJSON(page, source)).toBe('{"editor.tabSize":2}');
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await source.press("Control+Enter");
  expect((await stored(page))["editor.tabSize"]).toBe(8);
  await page
    .getByRole("button", { name: "Reload saved file", exact: true })
    .click();
  await expect
    .poll(() => readJSON(page, source))
    .toBe('{\n  "editor.tabSize": 8\n}\n');
  await page.getByRole("link", { name: "Sessions view", exact: true }).click();
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  await composer.fill("keep draft");
  await chooseSetting(other, "Global editor Tab display width", "6");
  await expect(composer).toHaveCSS("tab-size", "6");
  await expect(composer).toHaveValue("keep draft");
  await chooseSetting(other, "Session editor Indentation size", "5");
  await composer.press("Home");
  await composer.press("Tab");
  await expect(composer).toHaveValue("     keep draft");
});

test("invalid stored files stay recoverable and mobile settings remain accessible without horizontal overflow", async ({
  page,
}) => {
  await page.addInitScript(() => localStorage.setItem("settings", '{"broken"'));
  await ready(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => {
    location.hash = "/sessions";
  });
  await settings(page);
  await expect(
    page.locator(".settings-page p[role=alert]:visible"),
  ).toContainText("Invalid settings file");
  await expect(
    page.getByLabel("Global editor Indentation size", { exact: true }),
  ).toBeDisabled();
  const source = await file(page);
  await expect.poll(() => readJSON(page, source)).toBe('{"broken"');
  await writeJSON(page, source, '{"editor.tabSize":4}');
  await source.press("Control+Enter");
  await expect(
    page.locator(".settings-page p[role=alert]:visible"),
  ).toHaveCount(0);
  await page
    .getByRole("navigation", { name: "Settings topics", exact: true })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
  await expect(
    page.getByLabel("Global editor Indentation size", { exact: true }),
  ).toBeEnabled();
  await chooseSetting(page, "Session editor Color palette", "monochrome");
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({
    path: "test-results/settings-mobile.png",
    fullPage: true,
  });
});

test("global settings update the readonly file viewer in place while conversation settings remain independent", async ({
  page,
  context,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem(
      "settings",
      JSON.stringify({
        "editor.tabSize": 4,
        "editor.colorPalette": "monochrome",
        "session.editor.tabSize": 2,
      }),
    ),
  );
  await page.setViewportSize({ width: 1844, height: 1000 });
  await ready(page);
  const editor = page.getByRole("complementary", { name: "Workspace editor" });
  await editor.getByRole("button", { name: "src", exact: true }).click();
  await editor.getByRole("button", { name: "main.go", exact: true }).click();
  await expect(editor.locator(".view-lines")).toContainText("fmt.Println", {
    timeout: 30000,
  });
  const tabWidth = () =>
    editor
      .locator(".view-line")
      .filter({ hasText: "fmt.Println" })
      .evaluate((el) => {
        const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        let node: Node | null;
        while ((node = walker.nextNode())) {
          const position = node.textContent!.indexOf("fmt");
          if (position < 0) continue;
          const range = document.createRange();
          range.setStart(node, position);
          range.setEnd(node, position + 1);
          return (
            range.getBoundingClientRect().left - el.getBoundingClientRect().left
          );
        }
        throw new Error("fmt text missing");
      });
  const initialWidth = await tabWidth();
  const other = await context.newPage();
  await ready(other);
  await settings(other);
  await chooseSetting(other, "Global editor Tab display width", "8");
  await chooseSetting(other, "Global editor Color palette", "cool");
  await expect.poll(tabWidth).toBeGreaterThan(initialWidth * 1.9);
  await expect(
    editor
      .locator(".view-line")
      .first()
      .locator("span")
      .filter({ hasText: "package" })
      .last(),
  ).toHaveCSS("color", "rgb(126, 143, 175)");
  await expect(editor.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await expect(
    editor.getByRole("tab", { name: "main.go", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toHaveCSS("tab-size", "2");
});

test("settings sections form compact responsive columns with aligned text and ordered metadata", async ({
  page,
}) => {
  await ready(page);
  await settings(page);
  const groups = page.locator(".settings-editor-groups > .settings-group");
  const geometry = () =>
    groups.evaluateAll((elements) =>
      elements.map((el) => {
        const box = el.getBoundingClientRect();
        return { x: box.x, y: box.y, width: box.width };
      }),
    );
  const wide = await geometry();
  expect(wide[0].y).toBe(wide[1].y);
  expect(wide[1].x).toBeGreaterThan(wide[0].x);
  const sectionWidth = await page
    .locator(".settings-page")
    .evaluate((el) =>
      parseFloat(
        getComputedStyle(el).getPropertyValue("--settings-column-width"),
      ),
    );
  expect(wide[0].width).toBe(sectionWidth);
  const alignment = await page
    .locator(".settings-editor-body")
    .evaluate((el) => {
      const title = el.querySelector("header h1")!.getBoundingClientRect();
      const sectionTitle = el
        .querySelector(".settings-group h2")!
        .getBoundingClientRect();
      const section = el.querySelector(".settings-group")!;
      return {
        delta: Math.abs(title.left - sectionTitle.left),
        outsidePadding: getComputedStyle(el.querySelector(":scope > header")!)
          .paddingLeft,
        radius: getComputedStyle(section).borderTopLeftRadius,
      };
    });
  expect(alignment.delta).toBeLessThan(0.1);
  expect(alignment.outsidePadding).toBe(alignment.radius);
  const expectFieldOrder = async (field: Locator, detailed = false) => {
    const positions = await field.evaluate(
      (el, detailed) =>
        [
          ".setting-title",
          ".setting-id",
          ".setting-summary",
          ".setting-control",
          ...(detailed ? [".setting-details"] : []),
        ].map(
          (selector) => el.querySelector(selector)!.getBoundingClientRect().top,
        ),
      detailed,
    );
    for (let index = 1; index < positions.length; index++)
      expect(positions[index]).toBeGreaterThan(positions[index - 1]);
  };
  await expectFieldOrder(page.locator(".setting-row").first());
  await expect(
    page.locator(".setting-row").first().locator(".setting-details"),
  ).toHaveCount(0);
  await page.screenshot({ path: "test-results/settings-columns.png" });
  await page.setViewportSize({ width: 900, height: 1000 });
  await expect
    .poll(async () => (await geometry())[0].x === (await geometry())[1].x)
    .toBe(true);
  const narrow = await geometry();
  expect(narrow[0].width).toBe(sectionWidth);
  expect(narrow[1].y).toBeGreaterThan(narrow[0].y);
  await page
    .getByRole("navigation", { name: "Settings topics" })
    .getByRole("link", { name: "General", exact: true })
    .click();
  const language = page
    .locator(".setting-row")
    .filter({ has: page.locator("code", { hasText: "ui.language" }) });
  await expectFieldOrder(language, true);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByRole("radiogroup", { name: "Appearance", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({ path: "test-results/settings-general-compact.png" });
});
