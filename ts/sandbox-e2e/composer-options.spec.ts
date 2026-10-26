import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
  permissions: ["clipboard-read", "clipboard-write"],
});

test("Tab mode segments inherit settings, preserve drafts and switch native indentation", async ({
  page,
}) => {
  await page.addInitScript(() => {
    // Seed only once, so a reload checks the override saved by the toggle.
    if (!localStorage.getItem("settings")) {
      localStorage.setItem(
        "settings",
        JSON.stringify({
          "editor.insertSpaces": false,
          "editor.indentSize": 4,
          "editor.tabSize": 8,
        }),
      );
    }
  });
  await page.goto("/sandbox.html#/sessions/session-1");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeEditable({ timeout: 45000 });
  await input.fill("keep draft");
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(0, 4));
  const trigger = page.getByRole("button", {
    name: "Session menu",
    exact: true,
  });
  const options = page.getByRole("toolbar", {
    name: "Editor options",
    exact: true,
  });
  const segments = page.getByRole("radiogroup", {
    name: "Tab input",
    exact: true,
  });
  const tab = segments.getByRole("radio", {
    name: "Tab character",
    exact: true,
  });
  const spaces = segments.getByRole("radio", { name: "Spaces", exact: true });
  await trigger.click();
  await expect(tab).toBeChecked();
  await expect(spaces).not.toBeChecked();
  await expect(segments).toHaveAttribute("data-size", "compact");
  expect((await segments.boundingBox())!.height).toBe(24);
  await spaces.check();
  await expect(spaces).toBeChecked();
  await expect(input).toHaveValue("keep draft");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => [
      el.selectionStart,
      el.selectionEnd,
    ]),
  ).toEqual([0, 4]);
  await expect(options).toBeVisible();
  await spaces.press("ArrowLeft");
  await expect(tab).toBeFocused();
  await expect(tab).toBeChecked();
  await tab.press("ArrowRight");
  await expect(spaces).toBeFocused();
  await expect(spaces).toBeChecked();
  await spaces.press("Escape");
  await expect(trigger).toBeFocused();
  await input.focus();
  await input.press("Home");
  await input.press("Tab");
  await expect(input).toHaveValue("    keep draft");
  await expect(input).toHaveCSS("tab-size", "8");
  await input.press("Shift+Tab");
  await expect(input).toHaveValue("keep draft");
  await trigger.click();
  await tab.check();
  await expect(tab).toBeChecked();
  await tab.press("Escape");
  await input.focus();
  await input.press("Home");
  await input.press("Tab");
  await expect(input).toHaveValue("\tkeep draft");
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toEqual({
    "editor.insertSpaces": false,
    "editor.indentSize": 4,
    "editor.tabSize": 8,
    "session.editor.insertSpaces": false,
  });
  await page.reload();
  await expect(input).toHaveValue("\tkeep draft");
  await trigger.click();
  await expect(tab).toBeChecked();
  await tab.press("Escape");
  await input.focus();
  await input.press("Home");
  await input.press("Shift+Tab");
  await expect(input).toHaveValue("keep draft");
});

for (const wordWrap of [true, false]) {
  test(`control characters remain visible through Korean IME composition (wrap=${wordWrap})`, async ({
    page,
    context,
  }) => {
    await page.addInitScript((wordWrap) => {
      localStorage.setItem(
        "settings",
        JSON.stringify({
          "composer.showControlCharacters": true,
          "composer.wordWrap": wordWrap,
        }),
      );
    }, wordWrap);
    await page.goto("/sandbox.html#/sessions/session-1");
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await expect(input).toBeEditable({ timeout: 45000 });
    const prefix = "before ";
    const suffix = "\t after\nnext line";
    await input.fill(prefix + suffix);
    await input.evaluate((el: HTMLTextAreaElement, position) => {
      el.focus();
      el.setSelectionRange(position, position);
    }, prefix.length);
    const editor = input.locator("..").locator("..");
    const markers = editor.locator(".editor-controls-surface");
    const tab = markers.locator('[data-symbol="→"]');
    await expect(markers).toBeVisible();
    const initialTab = await tab.boundingBox();
    const cdp = await context.newCDPSession(page);
    // Drive the browser's IME rather than dispatching composition events alone:
    // each update replaces the active composition and fires native input events.
    for (const text of ["ㄱ", "가", "각"]) {
      await cdp.send("Input.imeSetComposition", {
        text,
        selectionStart: text.length,
        selectionEnd: text.length,
      });
      await expect(editor).toHaveAttribute("data-composing", "true");
      await expect(input).toHaveValue(prefix + text + suffix);
      await expect(markers).toBeVisible();
      await expect(tab).toBeVisible();
      // The line-end anchor is zero-width; its visible glyph is a pseudo-element.
      await expect(markers.locator('[data-symbol="¶"]')).toHaveCSS(
        "visibility",
        "visible",
      );
      await expect(markers.locator('[data-symbol="·"]')).toHaveCount(3);
      await expect(markers.locator(".editor-control-line").first()).toHaveText(
        prefix + text + "\t after\u200b",
      );
      expect((await tab.boundingBox())!.x).toBeGreaterThan(initialTab!.x);
      await expect(tab).toHaveCSS("pointer-events", "none");
      expect(
        await tab.evaluate((el) => getComputedStyle(el, "::after").content),
      ).toBe('"→"');
    }
    await cdp.send("Input.insertText", { text: "각" });
    await expect(editor).toHaveAttribute("data-composing", "false");
    await expect(input).toHaveValue(prefix + "각" + suffix);
    await expect(markers).toBeVisible();
    await input.press("Control+a");
    await input.press("Control+c");
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      prefix + "각" + suffix,
    );
    await cdp.detach();
  });
}

test("composer menu toggles stay open, preserve text and selection, and persist real display settings", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/sessions/session-1");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeEditable({ timeout: 45000 });
  const text = "first \t line\n" + "abcdefghij ".repeat(120);
  await input.fill(text);
  const original = await input.elementHandle();
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(2, 9));
  const trigger = page.getByRole("button", {
    name: "Session menu",
    exact: true,
  });
  await trigger.click();
  const options = page.getByRole("toolbar", {
    name: "Editor options",
    exact: true,
  });
  const controls = options.getByRole("button", {
    name: "Show control characters",
    exact: true,
  });
  const wrap = options.getByRole("button", { name: "Word wrap", exact: true });
  await expect(controls).toHaveAttribute("aria-pressed", "false");
  await expect(wrap).toHaveAttribute("aria-pressed", "true");
  await controls.click();
  await expect(controls).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("menu", { name: "Session menu", exact: true }),
  ).toBeVisible();
  const markers = page.locator(".editor-controls-surface");
  await expect(markers).toBeVisible();
  await expect(markers.locator('[data-symbol="→"]')).toHaveCount(1);
  await expect(markers.locator('[data-symbol="¶"]')).toHaveCount(1);
  await expect(input).toHaveValue(text);
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => [
      el.selectionStart,
      el.selectionEnd,
    ]),
  ).toEqual([2, 9]);
  await wrap.click();
  await expect(wrap).toHaveAttribute("aria-pressed", "false");
  await expect(input).toHaveAttribute("wrap", "off");
  await expect(input).toHaveCSS("white-space", "pre");
  const baseline = await input.evaluate((el) =>
    parseFloat(
      getComputedStyle(el).getPropertyValue("--composer-editor-height"),
    ),
  );
  await expect
    .poll(() => input.evaluate((el) => el.getBoundingClientRect().height))
    .toBeCloseTo(baseline, 0);
  // The still-open popup follows its toolbar as wrapping changes input height.
  await expect
    .poll(() =>
      trigger.evaluate((button) => {
        const toolbar = button
          .closest(".composer-toolbar")!
          .getBoundingClientRect();
        const anchor = button.getBoundingClientRect();
        const popup = document
          .querySelector(".action-menu-popup")!
          .getBoundingClientRect();
        return Math.abs(
          anchor.top - popup.bottom - 2 * (anchor.top - toolbar.top),
        );
      }),
    )
    .toBeLessThan(1);
  await expect
    .poll(() => input.evaluate((el) => el.scrollWidth > el.clientWidth))
    .toBe(true);
  expect(await input.evaluate((el, old) => el === old, original)).toBe(true);
  await wrap.press("ArrowLeft");
  await expect(controls).toBeFocused();
  await controls.press("ArrowRight");
  await expect(wrap).toBeFocused();
  await wrap.press("Escape");
  await expect(options).toHaveCount(0);
  await input.focus();
  await input.press("Control+a");
  await input.press("Control+c");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(text);
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toMatchObject({
    "composer.showControlCharacters": true,
    "composer.wordWrap": false,
  });
  await page.reload();
  await expect(input).toHaveValue(text);
  await expect(input).toHaveAttribute("wrap", "off");
  await expect(markers).toBeVisible();
  await trigger.click();
  await wrap.click();
  await expect(input).toHaveAttribute("wrap", "soft");
  await expect
    .poll(() => input.evaluate((el) => el.scrollWidth <= el.clientWidth + 1))
    .toBe(true);
  await controls.click();
  await expect(markers).toHaveCount(0);
  await expect(input).toHaveValue(text);
});
