import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("session heading shares the card identity, hides desktop Back, and puts the terminal in its menu", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const header = page.locator(".conversation > header");
  await expect(header.locator(".session-title")).toHaveText(
    "Project checklist",
    { timeout: 45000 },
  );
  await expect(header.locator(".session-alias")).toHaveText("session-1");
  await expect(
    header.getByRole("button", { name: "Back to sessions", exact: true }),
  ).toBeHidden();
  const logo = (await header
    .getByRole("img", { name: "Claude", exact: true })
    .boundingBox())!;
  const title = (await header.locator(".session-title").boundingBox())!;
  const alias = (await header.locator(".session-alias").boundingBox())!;
  expect(logo.x + logo.width).toBeLessThan(title.x);
  expect(alias.x).toBe(title.x);
  expect(alias.y).toBeGreaterThan(title.y);
  await expect(header.locator(".terminal-toggle")).toHaveCount(0);
  const trigger = header.getByRole("button", {
    name: "Session menu",
    exact: true,
  });
  await trigger.press("ArrowDown");
  const terminal = page.getByRole("menuitemcheckbox", {
    name: "Terminal",
    exact: true,
  });
  await expect(terminal).toBeFocused();
  await expect(terminal).toHaveAttribute("aria-checked", "false");
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await trigger.click();
  await terminal.click();
  await expect(
    page.getByRole("region", { name: "Workspace terminal", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Control+Backquote");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    header.getByRole("button", { name: "Back to sessions", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Session list", exact: true }),
  ).toBeHidden();
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
  expect((await input.boundingBox())!.height).toBe(baseline);
  await page.setViewportSize({ width: 390, height: 844 });
  await input.fill("line\n".repeat(40));
  expect((await input.boundingBox())!.height).toBeLessThanOrEqual(844 * 0.35);
  await expect
    .poll(() =>
      input.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(1);
});
