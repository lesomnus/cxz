import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("conversation starts at the transcript and Terminal lives in the menu on desktop and mobile", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await expect(page.locator(".conversation > header")).toHaveCount(0);
  const area = (await page.locator(".transcript-area").boundingBox())!;
  const conversation = (await page.locator(".conversation").boundingBox())!;
  expect(area.y).toBe(conversation.y);
  const toolbar = page.locator(".composer-toolbar");
  await expect(
    toolbar.getByRole("button", { name: "Terminal", exact: true }),
  ).toHaveCount(0);
  const menu = toolbar.getByRole("button", {
    name: "Session menu",
    exact: true,
  });
  const terminal = page.getByRole("menuitemcheckbox", {
    name: "Terminal",
    exact: true,
  });
  await menu.click();
  await expect(terminal).toHaveAttribute("aria-checked", "false");
  await expect(terminal.locator("kbd")).toHaveText(["Ctrl", "`"]);
  for (const key of await terminal.locator("kbd").all()) {
    await expect(key).toHaveCSS("font-size", "11px");
    await expect(key).toHaveCSS("font-family", /monospace/);
    await expect(key).toHaveCSS("border-bottom-width", "2px");
  }
  const labelBox = (await terminal
    .locator(".action-menu-item-label")
    .boundingBox())!;
  const shortcutBox = (await terminal.locator("kbd").first().boundingBox())!;
  expect(shortcutBox.x).toBeGreaterThan(labelBox.x + labelBox.width);
  const backtickBox = (await terminal.locator("kbd").last().boundingBox())!;
  expect(backtickBox.x).toBeGreaterThan(shortcutBox.x + shortcutBox.width);
  await terminal.click();
  const panel = page.getByRole("region", {
    name: "Workspace terminal",
    exact: true,
  });
  await expect(panel).toBeVisible();
  await expect(panel.locator(".xterm-helper-textarea")).toBeFocused();
  await menu.click();
  await expect(terminal).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("Escape");
  await page.keyboard.press("Control+Backquote");
  await expect(panel).toBeHidden();
  await expect(input).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(menu).toBeVisible();
  await menu.click();
  await expect(terminal).toBeVisible();
  await terminal.click();
  await expect(panel).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Session list", exact: true }),
  ).toBeHidden();
  await page.screenshot({ path: "test-results/composer-terminal-mobile.png" });
});

test("inline ticks pair and overtype while opening fences still pair as blocks", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await input.fill("Use ");
  await input.press("`");
  await expect(input).toHaveValue("Use ``");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(5);
  await input.press("Control+z");
  await expect(input).toHaveValue("Use ");
  await input.press("`");
  await input.pressSequentially("code`");
  await expect(input).toHaveValue("Use `code`");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(10);
  await expect(page.locator(".editor-inline-code")).toHaveText("`code`");
  await input.fill("");
  await input.pressSequentially("```");
  await expect(input).toHaveValue("```\n\n```");
});

test("overflow expands the input and last-line edits follow the bottom without pulling earlier edits down", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  const baseline = (await input.boundingBox())!.height;
  await input.fill(
    Array.from({ length: 40 }, (_, i) => `line ${i}`).join("\n"),
  );
  await expect(input).toHaveAttribute("data-expanded", "true");
  expect((await input.boundingBox())!.height).toBe(baseline * 3);
  await expect
    .poll(() =>
      input.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(1);
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.scrollTop = 0;
    el.setSelectionRange(el.value.length, el.value.length);
  });
  await input.pressSequentially(" tail");
  await expect
    .poll(() =>
      input.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(1);
  await input.evaluate((el: HTMLTextAreaElement) => {
    el.setSelectionRange(2, 2);
    el.scrollTop = 0;
  });
  await input.pressSequentially("x");
  expect(await input.evaluate((el) => el.scrollTop)).toBe(0);
  await input.fill("short");
  await expect(input).toHaveAttribute("data-expanded", "false");
  await expect
    .poll(async () => (await input.boundingBox())!.height)
    .toBe(baseline);
  await page.setViewportSize({ width: 390, height: 844 });
  await input.fill("line\n".repeat(40));
  expect((await input.boundingBox())!.height).toBeLessThanOrEqual(844 * 0.35);
  await expect
    .poll(() =>
      input.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(1);
});
