import { test, expect, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1200, height: 420 },
});

test("panel fades track hidden edges and scroll intensity without covering the heading or handle", async ({
  page,
}) => {
  await page.goto("/sandbox.html");
  const panel = page.getByRole("complementary", {
    name: "Session list",
    exact: true,
  });
  await expect(panel.locator(".tree-session").first()).toBeVisible({
    timeout: 45000,
  });
  const list = panel.locator(".panel-scroll-content");
  const top = panel.locator(".panel-scroll-fade-top");
  const bottom = panel.locator(".panel-scroll-fade-bottom");
  const depth = (edge: typeof top) =>
    edge.evaluate((el) => el.getBoundingClientRect().height);
  await expect(top).toHaveCSS("opacity", "0");
  await expect(bottom).toHaveCSS("opacity", "1");
  await list.evaluate((el) => {
    el.scrollTop = 48;
  });
  await expect(top).toHaveCSS("opacity", "1");
  // Wait for motion to settle before comparing slow and fast movement.
  await page.waitForTimeout(900);
  const idle = await depth(top);
  const motion = await list.evaluate(async (el) => {
    const fade = el.parentElement!.querySelector<HTMLElement>(
      ".panel-scroll-fade-top",
    )!;
    const frame = () =>
      new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    const sample = () => fade.getBoundingClientRect().height;
    const slow: number[] = [],
      fast: number[] = [];
    for (let i = 0; i < 10; i++) {
      el.scrollTop += 2;
      await frame();
      slow.push(sample());
    }
    await new Promise((resolve) => setTimeout(resolve, 900));
    el.scrollTop += 80;
    for (let i = 0; i < 12; i++) {
      await frame();
      fast.push(sample());
    }
    return { slow, fast };
  });
  expect(Math.max(...motion.slow)).toBeGreaterThan(idle);
  expect(Math.max(...motion.fast)).toBeGreaterThan(
    Math.max(...motion.slow) + 4,
  );
  await expect.poll(() => depth(top)).toBeCloseTo(idle, 0);

  const bottomIdle = await depth(bottom);
  const peak = await list.evaluate(async (el) => {
    const fade = el.parentElement!.querySelector<HTMLElement>(
      ".panel-scroll-fade-bottom",
    )!;
    el.scrollTop -= 80;
    let peak = 0;
    for (let i = 0; i < 12; i++) {
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      );
      peak = Math.max(peak, fade.getBoundingClientRect().height);
    }
    return peak;
  });
  expect(peak).toBeGreaterThan(bottomIdle + 4);
  // Reversing away from the end restores the full resting fade; near the end
  // its depth was limited by the amount of hidden content.
  await expect.poll(() => depth(bottom)).toBeCloseTo(idle, 0);
  await list.evaluate((el) => {
    el.scrollTop = 24;
  });
  await expect.poll(() => depth(top)).toBeCloseTo(24, 0);
  const layers = await panel.evaluate((el) => {
    const region = el.querySelector<HTMLElement>(".panel-scroll-region")!;
    const thumb = region.querySelector<HTMLElement>(".panel-scroll-thumb")!;
    const fade = region.querySelector<HTMLElement>(".panel-scroll-fade-top")!;
    const header = el.querySelector("header")!.getBoundingClientRect();
    fade.style.pointerEvents = "auto";
    const box = thumb.getBoundingClientRect();
    const fadeBox = fade.getBoundingClientRect();
    const overlapsHandle = box.y + 2 < fadeBox.bottom;
    const handleAbove = thumb.contains(
      document.elementFromPoint(box.x + box.width / 2, box.y + 2),
    );
    fade.style.pointerEvents = "none";
    return {
      overlapsHandle,
      handleAbove,
      belowHeading: fade.getBoundingClientRect().top >= header.bottom,
      background: getComputedStyle(fade).backgroundImage,
      panel: getComputedStyle(el).backgroundColor,
    };
  });
  expect(layers.overlapsHandle).toBe(true);
  expect(layers.handleAbove).toBe(true);
  expect(layers.belowHeading).toBe(true);
  expect(layers.background).toContain(layers.panel);

  const thumb = panel.getByRole("scrollbar", { name: "Panel scroll" });
  await thumb.focus();
  await thumb.press("End");
  await expect(bottom).toHaveCSS("opacity", "0");
  await thumb.press("Home");
  await expect(top).toHaveCSS("opacity", "0");
  await page.setViewportSize({ width: 1200, height: 1400 });
  await expect(thumb).toHaveCount(0);
  await expect(top).toHaveCSS("opacity", "0");
  await expect(bottom).toHaveCSS("opacity", "0");
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
  expect((await panel.boundingBox())!.width).toBe(160);
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
      (await panel.boundingBox())!.width -
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
