import { test, expect, devices, type Page } from "@playwright/test";
import { chooseSetting, expectInherited } from "./settings-controls";
import { disableTerminalWebGL } from "../test-support/terminal";
test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 2000, height: 1000 },
});
async function ready(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible({ timeout: 45000 });
}
async function general(page: Page) {
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "General", exact: true }),
  ).toBeVisible();
}
async function editor(page: Page) {
  await page
    .getByRole("navigation", { name: "Settings topics" })
    .getByRole("link", { name: "Editor", exact: true })
    .click();
}
test("General is first, shared menus preserve the current text position and system appearance follows the OS without resetting JSON Undo", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await ready(page);
  await general(page);
  const topics = page.getByRole("navigation", { name: "Settings topics" });
  await expect(topics.getByRole("link")).toHaveText(["General", "Editor"]);
  await expect(
    page.getByRole("button", { name: "Language", exact: true }),
  ).toHaveCount(0);
  const source = page.getByRole("textbox", {
    name: "settings.json",
    exact: true,
  });
  await expect(page.locator(".settings-file .monaco-editor")).toBeVisible({
    timeout: 30000,
  });
  const original = await source.elementHandle();
  await page.evaluate(() => navigator.clipboard.writeText('{"unsaved":true}'));
  await source.press("Control+a");
  await source.press("Control+v");
  const appearance = page.getByRole("combobox", {
    name: "Appearance",
    exact: true,
  });
  await expect(appearance).toHaveAttribute("data-muted", "true");
  const before = (await appearance.locator(".meta-value").boundingBox())!;
  await appearance.click();
  const popup = page.getByRole("listbox");
  const after = (await popup
    .locator(".setting-current .meta-value")
    .boundingBox())!;
  expect(Math.abs(after.x - before.x)).toBeLessThan(0.1);
  expect(Math.abs(after.y - before.y)).toBeLessThan(0.1);
  await page.keyboard.press("Escape");
  await expect(appearance).toBeFocused();
  await chooseSetting(page, "Appearance", "light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(page.locator("html")).toHaveCSS(
    "background-color",
    "rgb(250, 250, 250)",
  );
  await expect(page.locator(".settings-file .monaco-editor")).toHaveCSS(
    "background-color",
    "rgb(250, 250, 250)",
  );
  expect(await source.evaluate((el, old) => el === old, original)).toBe(true);
  await expect(page.locator(".settings-file .view-lines")).toContainText(
    "unsaved",
  );
  await source.press("Control+z");
  await expect(page.locator(".settings-file .view-lines")).not.toContainText(
    "unsaved",
  );
  await page.emulateMedia({ colorScheme: "dark" });
  await chooseSetting(page, "Appearance", "system");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  expect(
    await page.evaluate(
      () => JSON.parse(localStorage.getItem("settings")!)["ui.theme"],
    ),
  ).toBe("system");
  await page.screenshot({ path: "test-results/settings-general-light.png" });
  await page.reload();
  await general(page);
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await chooseSetting(page, "Appearance", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});
test("sliders reset to per-field inheritance and equal-width Tab input cells animate the selection without shadows", async ({
  page,
}) => {
  await ready(page);
  await general(page);
  await editor(page);
  const slider = page.getByRole("slider", {
    name: "Session editor Indentation size",
    exact: true,
  });
  await expect(slider).toHaveAttribute("min", "0");
  await expect(slider).toHaveAttribute("max", "8");
  await expectInherited(page, "Session editor Indentation size");
  await chooseSetting(page, "Global editor Indentation size", "5");
  await expect(slider).toHaveAttribute("aria-valuetext", "Inherited: 5");
  await expect(slider.locator("..").locator("output")).toHaveText("5");
  await chooseSetting(page, "Session editor Indentation size", "8");
  await chooseSetting(page, "Session editor Indentation size", "");
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("settings")!)[
          "session.editor.indentSize"
        ],
    ),
  ).toBeUndefined();
  const tabs = page.getByRole("radiogroup", {
    name: "Session editor Tab input",
    exact: true,
  });
  const widths = await tabs
    .locator(".segment")
    .evaluateAll((els) => els.map((el) => el.getBoundingClientRect().width));
  expect(Math.max(...widths) - Math.min(...widths)).toBeLessThan(0.1);
  await expect(tabs.locator(".segment-indicator")).toHaveCSS(
    "box-shadow",
    "none",
  );
  await tabs.locator('input[value="true"]').check();
  await page.keyboard.press("ArrowRight");
  await expect(tabs).toHaveAttribute("data-value", "false");
  await expect
    .poll(() =>
      tabs
        .locator(".segment-indicator")
        .evaluate((el) =>
          Math.round(
            el.getBoundingClientRect().left -
              el.parentElement!.getBoundingClientRect().left,
          ),
        ),
    )
    .toBe(Math.round(2 + widths[0] * 2));
  await chooseSetting(page, "Session editor Tab input", "");
  const palette = page.getByRole("combobox", {
    name: "Session editor Color palette",
    exact: true,
  });
  await expect(palette).toHaveText("Muted");
  await expect(palette).toHaveAttribute("data-muted", "true");
  await palette.click();
  await expect(page.getByRole("listbox")).not.toContainText(
    /Default|Inherit global/,
  );
  await page.keyboard.press("End");
  if (
    await page
      .getByRole("listbox")
      .evaluate((el) => el.classList.contains("opens-up"))
  )
    await page.keyboard.press("ArrowUp");
  await page.keyboard.press("Enter");
  await expect(palette).toHaveText("Warm");
  await expect(palette).toHaveAttribute("data-muted", "false");
  await chooseSetting(page, "Session editor Color palette", "");
  await expect(palette).toHaveAttribute("data-muted", "true");
  await page.screenshot({ path: "test-results/settings-controls.png" });
});

test("cross-tab theme changes retain the composer Undo, selected file and running terminal connection", async ({
  page,
  context,
}) => {
  await disableTerminalWebGL(page);
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const original = await input.elementHandle();
  await input.fill("keep draft");
  const viewer = page.getByRole("complementary", {
    name: "Workspace editor",
    exact: true,
  });
  await viewer.getByRole("button", { name: "README.md", exact: true }).click();
  await expect(viewer.locator(".monaco-editor")).toBeVisible({
    timeout: 30000,
  });
  const fileInput = await viewer
    .locator(".monaco-editor textarea")
    .elementHandle();
  await page.keyboard.press("Control+Backquote");
  const shell = page.getByRole("region", {
    name: "Workspace terminal",
    exact: true,
  });
  await expect(shell).toContainText("sandbox@project-1:/workspace$");
  const terminal = await shell
    .locator(".xterm-helper-textarea")
    .elementHandle();
  await page.keyboard.type("echo keep-terminal");
  await page.keyboard.press("Enter");
  await expect(shell).toContainText("keep-terminal");
  // Establish composer history after terminal typing and Monaco initialization,
  // so the test measures the theme change rather than other native editor edits.
  await input.press("End");
  await input.press("Tab");
  await input.press("Control+z");
  await expect(input).toHaveValue("keep draft");
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue("keep draft  ");
  const other = await context.newPage();
  await ready(other);
  await general(other);
  await chooseSetting(other, "Appearance", "light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(viewer.locator(".monaco-editor")).toHaveCSS(
    "background-color",
    "rgb(250, 250, 250)",
  );
  await expect(shell.locator(".xterm-viewport")).toHaveCSS(
    "background-color",
    "rgb(251, 251, 251)",
  );
  expect(
    await viewer
      .locator(".monaco-editor textarea")
      .evaluate((el, old) => el === old, fileInput),
  ).toBe(true);
  expect(
    await shell
      .locator(".xterm-helper-textarea")
      .evaluate((el, old) => el === old, terminal),
  ).toBe(true);
  await expect(shell).toContainText("keep-terminal");
  await shell
    .getByRole("button", { name: "Hide terminal", exact: true })
    .click();
  expect(await input.evaluate((el, old) => el === old, original)).toBe(true);
  await expect(input).toHaveValue("keep draft  ");
  await input.press("Control+z");
  await expect(input).toHaveValue("keep draft");
  await page.screenshot({ path: "test-results/session-light.png" });
});
