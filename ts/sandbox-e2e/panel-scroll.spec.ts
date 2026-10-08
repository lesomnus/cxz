import { test, expect, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1200, height: 420 },
});

test("compact panels keep their heading fixed and share the conversation handle's hover and drag behavior", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const panel = page.getByRole("complementary", {
    name: "Session list",
    exact: true,
  });
  const list = panel.locator(".panel-scroll-content");
  await expect(panel.locator(".tree-session").first()).toBeVisible({
    timeout: 45000,
  });
  expect((await panel.boundingBox())!.width).toBe(144);
  const thumb = panel.getByRole("scrollbar", { name: "Panel scroll" });
  await expect(thumb).toBeVisible();
  const handle = thumb.locator(".scroll-handle");
  const heading = panel.locator("header");
  const headingY = (await heading.boundingBox())!.y;
  await page.mouse.move(1000, 100);
  await expect(handle).toHaveCSS("opacity", "0");
  await heading.hover();
  await expect(handle).toHaveCSS("opacity", "0.7");
  const initial = (await handle.boundingBox())!;
  const scrollBox = (await panel
    .locator(".panel-scroll-region")
    .boundingBox())!;
  await page.mouse.move(initial.x + initial.width / 2, scrollBox.y + 16);
  await expect
    .poll(async () => (await handle.boundingBox())!.width)
    .toBeGreaterThan(initial.width * 1.9);
  expect(
    (await panel.boundingBox())!.x +
      144 -
      ((await handle.boundingBox())!.x + (await handle.boundingBox())!.width),
  ).toBeGreaterThan(4);
  const conversationHandle = page.locator(".transcript-area .scroll-handle");
  await expect(conversationHandle).toBeAttached();
  expect(
    await handle.evaluate((el) => getComputedStyle(el).backgroundColor),
  ).toBe(
    await conversationHandle.evaluate(
      (el) => getComputedStyle(el).backgroundColor,
    ),
  );
  const bounds = (await thumb.boundingBox())!;
  await page.mouse.move(
    bounds.x + bounds.width / 2,
    bounds.y + bounds.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    bounds.x + bounds.width / 2,
    scrollBox.y + scrollBox.height,
  );
  await page.mouse.up();
  await expect
    .poll(() => list.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(30);
  expect((await heading.boundingBox())!.y).toBe(headingY);
  await thumb.focus();
  await thumb.press("Home");
  await expect.poll(() => list.evaluate((el) => el.scrollTop)).toBe(0);
  await thumb.press("End");
  await expect
    .poll(() =>
      list.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(1);
  expect((await heading.boundingBox())!.y).toBe(headingY);
  await page.screenshot({
    path: "test-results/panel-scroll.png",
    fullPage: true,
  });
});
