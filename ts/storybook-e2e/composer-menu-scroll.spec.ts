import { expect, test } from "@playwright/test";

for (const deviceScaleFactor of [1, 1.5]) {
  test.describe(`real transcript backdrop at scale ${deviceScaleFactor}`, () => {
    test.use({ deviceScaleFactor });
    test("rail removes real conversation glyph detail while keeping the menu glass", async ({
      page,
    }) => {
      await page.setViewportSize({ width: 1040, height: 760 });
      await page.goto(
        "/iframe.html?id=conversation-composer--menu-over-transcript&viewMode=story",
      );
      await page
        .getByRole("button", { name: "Session menu", exact: true })
        .click();
      const popup = page.locator(".action-menu-popup");
      await expect(popup.locator("[data-menu-frame]")).toHaveCSS(
        "filter",
        "none",
      );
      await expect(popup).toHaveCSS("backdrop-filter", "none");
      expect(
        await popup.evaluate(
          (node) => getComputedStyle(node, "::before").backdropFilter,
        ),
      ).not.toBe("none");
      const area = popup.locator('[data-scroll-area="root"]');
      const rail = area.locator('[data-scroll-area="rail"]');
      await area.hover();
      await expect
        .poll(() =>
          rail.evaluate((node) => getComputedStyle(node, "::before").opacity),
        )
        .toBe("1");
      await page.evaluate(
        () =>
          new Promise((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(resolve)),
          ),
      );
      const clip = await rail.evaluate((node) => {
        const bounds = node.getBoundingClientRect();
        const thumb = node
          .querySelector('[role="scrollbar"]')!
          .getBoundingClientRect();
        const walker = document.createTreeWalker(
          document.querySelector(".transcript .markdown")!,
          NodeFilter.SHOW_TEXT,
        );
        let text: Node | null;
        while ((text = walker.nextNode())) {
          const range = document.createRange();
          range.selectNodeContents(text);
          const line = range.getBoundingClientRect();
          if (line.bottom < thumb.bottom || line.top > bounds.bottom) continue;
          for (
            let index = 0;
            index < (text.textContent?.length ?? 0);
            index++
          ) {
            if (!/\w/.test(text.textContent![index])) continue;
            range.setStart(text, index);
            range.setEnd(text, index + 1);
            const glyph = range.getBoundingClientRect();
            if (
              glyph.left >= bounds.left + 2 &&
              glyph.right <= bounds.right - 2 &&
              glyph.top >= thumb.bottom + 5 &&
              glyph.bottom <= bounds.bottom - 5
            )
              return {
                x: Math.floor(glyph.x),
                y: Math.floor(glyph.y),
                width: Math.ceil(glyph.width),
                height: Math.ceil(glyph.height),
              };
          }
        }
        throw new Error(
          "No actual conversation glyph is behind the uncovered rail",
        );
      });
      const contrast = async (png: Buffer) =>
        page.evaluate(
          async ({ base64, region }) => {
            const image = new Image();
            image.src = `data:image/png;base64,${base64}`;
            await image.decode();
            const canvas = document.createElement("canvas");
            const scale = image.width / innerWidth;
            canvas.width = Math.ceil(region.width * scale);
            canvas.height = Math.ceil(region.height * scale);
            const context = canvas.getContext("2d")!;
            context.drawImage(
              image,
              region.x * scale,
              region.y * scale,
              region.width * scale,
              region.height * scale,
              0,
              0,
              canvas.width,
              canvas.height,
            );
            const pixels = context.getImageData(
              0,
              0,
              canvas.width,
              canvas.height,
            ).data;
            let min = 255,
              max = 0;
            for (let index = 0; index < pixels.length; index += 4) {
              const light =
                (pixels[index] + pixels[index + 1] + pixels[index + 2]) / 3;
              min = Math.min(min, light);
              max = Math.max(max, light);
            }
            return max - min;
          },
          { base64: png.toString("base64"), region: clip },
        );
      // Capture the viewport first: partial captures can alter backdrop compositing.
      const blurred = await contrast(await page.screenshot());
      await page.addStyleTag({
        content:
          '[data-scroll-area="rail"]::before { backdrop-filter: none !important; -webkit-backdrop-filter: none !important; }',
      });
      await page.evaluate(
        () =>
          new Promise((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(resolve)),
          ),
      );
      const sharp = await contrast(await page.screenshot());
      expect(sharp).toBeGreaterThan(100);
      expect(blurred).toBeLessThan(sharp / 2);
    });
  });
}

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
