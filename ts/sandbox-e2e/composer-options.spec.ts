import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
  permissions: ["clipboard-read", "clipboard-write"],
});

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
