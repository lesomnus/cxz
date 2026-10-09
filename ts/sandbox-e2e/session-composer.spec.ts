import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("conversation starts at the transcript and the terminal toggle sits beside Send on desktop and mobile", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await expect(page.locator(".conversation > header")).toHaveCount(0);
  const area = (await page.locator(".transcript-area").boundingBox())!;
  const conversation = (await page.locator(".conversation").boundingBox())!;
  expect(area.y).toBe(conversation.y);
  const terminal = page.getByRole("button", {
    name: "Terminal",
    exact: true,
  });
  const toolbar = page.locator(".composer-toolbar");
  await expect(
    toolbar.getByRole("button", { name: "Terminal", exact: true }),
  ).toBeVisible();
  await expect(terminal).toHaveAttribute("aria-pressed", "false");
  const send = page.getByRole("button", { name: "Send", exact: true });
  const terminalBox = (await terminal.boundingBox())!;
  const sendBox = (await send.boundingBox())!;
  expect(terminalBox.x + terminalBox.width).toBeLessThan(sendBox.x);
  expect(terminalBox.y).toBe(sendBox.y);
  expect(terminalBox.height).toBe(sendBox.height);
  expect(terminalBox.width).toBe(sendBox.width);
  await terminal.click();
  await expect(
    page.getByRole("region", { name: "Workspace terminal", exact: true }),
  ).toBeVisible();
  await expect(terminal).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("Control+Backquote");
  await expect(terminal).toHaveAttribute("aria-pressed", "false");
  await expect(input).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".conversation > header")).toHaveCount(0);
  await expect(terminal).toBeVisible();
  const mobileTerminal = (await terminal.boundingBox())!;
  const mobileSend = (await send.boundingBox())!;
  expect(mobileTerminal.x + mobileTerminal.width).toBeLessThan(mobileSend.x);
  expect(mobileTerminal.y).toBe(mobileSend.y);
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
