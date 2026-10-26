import { expect, test } from "@playwright/test";

test("composer rail blurs its backdrop beyond the thin track line", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
  );
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  const popup = page.locator(".action-menu-popup");
  await expect(popup.locator("[data-menu-frame]")).toHaveCSS("filter", "none");
  const area = popup.locator('[data-scroll-area="root"]');
  const rail = area.locator('[data-scroll-area="rail"]');
  await area.hover();
  await expect
    .poll(() =>
      rail.evaluate((node) => getComputedStyle(node, "::before").opacity),
    )
    .toBe("1");
  await page.evaluate(() => {
    const background = document.createElement("div");
    background.style.cssText =
      "position:fixed;inset:0;z-index:1000;pointer-events:none;background:repeating-linear-gradient(0deg,#222 0 2px,#ddd 2px 4px)";
    document.body.append(background);
  });
  const bounds = (await rail.boundingBox())!;
  const clips = [0.25, 0.75].map((fraction) => ({
    x: Math.round(bounds.x + bounds.width * fraction - 1),
    y: Math.round(bounds.y + bounds.height - 30),
    width: 2,
    height: 20,
  }));
  const blurred = [];
  for (const clip of clips) blurred.push(await page.screenshot({ clip }));
  await page.addStyleTag({
    content:
      '[data-scroll-area="rail"]::before {backdrop-filter:none!important;-webkit-backdrop-filter:none!important;}',
  });
  for (const [index, clip] of clips.entries())
    expect((await page.screenshot({ clip })).equals(blurred[index])).toBe(
      false,
    );
});

for (const width of [1280, 390]) {
  test(`composer menu scrollbar stays outside its border at width ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 620 });
    await page.goto(
      "/iframe.html?id=conversation-composer--file-uploads&viewMode=story",
    );
    const trigger = page.getByRole("button", {
      name: "Session menu",
      exact: true,
    });
    await trigger.click();
    const popup = page.locator(".action-menu-popup");
    const area = popup.locator('[data-scroll-area="root"]');
    const viewport = area.locator('[data-scroll-area="viewport"]');
    const thumb = area.getByRole("scrollbar");
    const handle = area.locator('[data-scroll-area="handle"]');
    await expect(thumb).toBeVisible();
    await expect(popup.locator("[data-menu-frame]")).toHaveCSS(
      "mask-image",
      "none",
    );
    await area.hover();
    const frame = (await popup.boundingBox())!;
    const bar = (await handle.boundingBox())!;
    expect(bar.x).toBeGreaterThan(frame.x + frame.width);
    expect(bar.x + bar.width).toBeLessThan(width);
    const footer = popup.locator('[data-menu-fixed="footer"]');
    const fixed = await footer.boundingBox();
    await page.mouse.move(bar.x + bar.width / 2, bar.y + bar.height / 2);
    await expect(handle).toHaveCSS("opacity", "1");
    expect(
      await handle.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return node.parentElement!.contains(
          document.elementFromPoint(
            box.x + box.width / 2,
            box.y + box.height / 2,
          ),
        );
      }),
    ).toBe(true);
    const rail = (await area
      .locator('[data-scroll-area="rail"]')
      .boundingBox())!;
    await page.mouse.down();
    await page.mouse.move(bar.x + bar.width / 2, rail.y + rail.height);
    await page.mouse.up();
    await expect
      .poll(() =>
        viewport.evaluate(
          (node) => node.scrollHeight - node.clientHeight - node.scrollTop,
        ),
      )
      .toBeLessThan(1);
    expect(await footer.boundingBox()).toEqual(fixed);
    const wrap = popup.getByRole("button", { name: "Word wrap", exact: true });
    await wrap.click();
    await expect(popup).toBeVisible();
    await thumb.press("Home");
    await expect
      .poll(() => viewport.evaluate((node) => node.scrollTop))
      .toBe(0);
    await page.keyboard.press("Escape");
    await expect(popup).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });
}
