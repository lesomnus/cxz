import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("event details grow only the trigger's exterior, toggle and replace in place, and dismiss from side margins", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/sessions/session-5");
  const trigger = page.locator(".detail-anchor").last();
  await expect(trigger).toContainText("Simulated usage limit", {
    timeout: 45000,
  });
  const pane = page.locator(".transcript");
  const geometry = await pane.evaluate((el) => ({
    top: el.scrollTop,
    height: el.scrollHeight,
  }));
  const before = await trigger.boundingBox();
  const text = await trigger.locator(".button-content").boundingBox();
  await trigger.click();
  const card = page.getByRole("dialog");
  await expect(card).toHaveCSS("opacity", "1");
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(trigger).toBeFocused();
  await expect(
    card.getByRole("button", { name: "Close details", exact: true }),
  ).toHaveCount(0);
  await expect(card).toContainText("usageLimitExceeded");
  const bounds = (await card.boundingBox())!;
  expect(bounds.x).toBe(before!.x);
  expect(bounds.width).toBe(before!.width);
  expect(bounds.y).toBeCloseTo(before!.y + before!.height - 4, 0);
  expect(await trigger.boundingBox()).toEqual(before);
  await expect
    .poll(() => trigger.locator(".button-content").boundingBox())
    .toEqual(text);
  expect(
    await pane.evaluate((el) => ({
      top: el.scrollTop,
      height: el.scrollHeight,
    })),
  ).toEqual(geometry);
  const hit = await trigger.evaluate((el) => {
    const rect = el.getBoundingClientRect();
    const target = document.elementFromPoint(
      rect.x + rect.width / 2,
      rect.bottom + 2,
    );
    return target === el || (!!target && el.contains(target));
  });
  expect(hit).toBe(true);
  await page.screenshot({ path: "test-results/anchored-details-desktop.png" });
  await trigger.click();
  await expect(card).toHaveCount(0);
  await trigger.press("Enter");
  await expect(card).toBeVisible();
  const previous = await card.getAttribute("id");
  const first = page.locator(".detail-anchor").first();
  await first.click();
  await expect(first).toHaveAttribute("aria-expanded", "true");
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator(`[id="${previous}"]`)).toHaveCount(0);
  await expect(card).toHaveCount(1);
  const column = (await page.locator(".transcript-content").boundingBox())!;
  const view = (await pane.boundingBox())!;
  await page.mouse.click(column.x - 16, view.y + 30);
  await expect(card).toHaveCount(0);
  await expect(first).toHaveAttribute("aria-expanded", "false");
  await page.setViewportSize({ width: 390, height: 844 });
  await trigger.click();
  await expect(card).toHaveCSS("opacity", "1");
  const mobile = (await card.boundingBox())!;
  expect(mobile.x).toBeGreaterThanOrEqual(0);
  expect(mobile.x + mobile.width).toBeLessThanOrEqual(390);
  expect(mobile.y + mobile.height).toBeLessThanOrEqual(844);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  await page.screenshot({ path: "test-results/anchored-details-mobile.png" });
  await page.keyboard.press("Escape");
  await expect(card).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("grouped tool details follow native scrolling, scroll their own output, and close when a virtualized trigger leaves view", async ({
  page,
}) => {
  // A short conversation leaves less room than this tool's combined details.
  await page.setViewportSize({ width: 1440, height: 360 });
  await page.goto("/sandbox.html#/sessions/session-3");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await input.fill("Inspect this work");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  const tool = page.locator(".tool-activity").last();
  await expect(tool).toHaveAttribute("data-state", "completed");
  await expect(page.locator(".conversation")).toHaveAttribute(
    "aria-description",
    /idle/,
  );
  await tool.click();
  const card = page.getByRole("dialog");
  await expect(card).toHaveCSS("opacity", "1");
  const bounds = (await card.boundingBox())!;
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(360);
  await expect(
    card.getByRole("heading", { name: "Output", exact: true }),
  ).toBeVisible();
  const pane = page.locator(".transcript");
  const top = await pane.evaluate((el) => el.scrollTop);
  await card.locator(".card-body").hover();
  await page.mouse.wheel(0, 80);
  await expect
    .poll(() => card.locator(".card-body").evaluate((el) => el.scrollTop))
    .toBeGreaterThan(0);
  expect(await pane.evaluate((el) => el.scrollTop)).toBe(top);
  await pane.evaluate((el) => {
    el.scrollTop -= 32;
  });
  await expect
    .poll(async () => {
      const source = (await tool.boundingBox())!;
      const details = (await card.boundingBox())!;
      return Math.abs(details.y - (source.y + source.height - 4));
    })
    .toBeLessThan(1);
  await pane.focus();
  await pane.evaluate((el) => {
    el.scrollTop = 0;
  });
  await expect(card).toHaveCount(0);
  await expect(pane).toBeFocused();
});
