import { expect, test } from "@playwright/test";

for (const deviceScaleFactor of [1, 1.5]) {
  test.describe(`real transcript backdrop at scale ${deviceScaleFactor}`, () => {
    test.use({ deviceScaleFactor });
    test("only the visible rail blurs real conversation text", async ({
      page,
    }) => {
      await page.setViewportSize({ width: 1040, height: 400 });
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
      const area = popup.locator('[data-scroll-area="root"]');
      const rail = area.locator('[data-scroll-area="rail"]');
      await area.hover();
      await expect
        .poll(() =>
          rail.evaluate((node) => getComputedStyle(node, "::before").opacity),
        )
        .toBe("1");
      const regions = await rail.evaluate((node) => {
        const bounds = node.getBoundingClientRect();
        const thumb = node
          .querySelector('[role="scrollbar"]')!
          .getBoundingClientRect();
        const width = parseFloat(getComputedStyle(node, "::before").width);
        const left = bounds.x + (bounds.width - width) / 2;
        const x = Math.ceil(left),
          right = Math.floor(left + width);
        const y = Math.ceil(thumb.bottom + 8);
        const height = Math.floor(bounds.bottom - 8) - y;
        const walker = document.createTreeWalker(
          document.querySelector(".transcript .markdown")!,
          NodeFilter.SHOW_TEXT,
        );
        let text: Node | null,
          overlapsText = false;
        while ((text = walker.nextNode())) {
          for (
            let index = 0;
            index < (text.textContent?.length ?? 0);
            index++
          ) {
            if (!/\w/.test(text.textContent![index])) continue;
            const range = document.createRange();
            range.setStart(text, index);
            range.setEnd(text, index + 1);
            const glyph = range.getBoundingClientRect();
            if (
              glyph.left < right &&
              glyph.right > x &&
              glyph.top >= y &&
              glyph.bottom <= y + height
            )
              overlapsText = true;
          }
        }
        if (!overlapsText)
          throw new Error(
            "No actual conversation text is behind the rail line",
          );
        return {
          line: { x, y, width: right - x, height },
          neighbors: [
            {
              x: Math.ceil(bounds.left),
              y,
              width: Math.floor(left) - Math.ceil(bounds.left),
              height,
            },
            {
              x: Math.ceil(left + width),
              y,
              width: Math.floor(bounds.right) - Math.ceil(left + width),
              height,
            },
          ],
        };
      });
      const sample = async (png: Buffer) =>
        page.evaluate(
          async ({ base64, regions }) => {
            const image = new Image();
            image.src = `data:image/png;base64,${base64}`;
            await image.decode();
            const scale = image.width / innerWidth;
            function pixels(region: {
              x: number;
              y: number;
              width: number;
              height: number;
            }) {
              const canvas = document.createElement("canvas");
              const x = Math.ceil(region.x * scale);
              const y = Math.ceil(region.y * scale);
              canvas.width = Math.floor((region.x + region.width) * scale) - x;
              canvas.height =
                Math.floor((region.y + region.height) * scale) - y;
              const context = canvas.getContext("2d")!;
              context.drawImage(
                image,
                x,
                y,
                canvas.width,
                canvas.height,
                0,
                0,
                canvas.width,
                canvas.height,
              );
              return context.getImageData(0, 0, canvas.width, canvas.height)
                .data;
            }
            let min = 255,
              max = 0;
            const line = pixels(regions.line);
            for (let index = 0; index < line.length; index += 4) {
              const light =
                (line[index] + line[index + 1] + line[index + 2]) / 3;
              min = Math.min(min, light);
              max = Math.max(max, light);
            }
            const neighbors = await Promise.all(
              regions.neighbors.map(async (region) => {
                const hash = await crypto.subtle.digest(
                  "SHA-256",
                  pixels(region),
                );
                return Array.from(new Uint8Array(hash), (byte) =>
                  byte.toString(16).padStart(2, "0"),
                ).join("");
              }),
            );
            return { contrast: max - min, neighbors };
          },
          { base64: png.toString("base64"), regions },
        );
      // Full viewport captures keep backdrop compositing stable at fractional DPR.
      const blurred = await sample(await page.screenshot());
      await page.addStyleTag({
        content:
          '[data-scroll-area="rail"]::before {backdrop-filter:none!important; -webkit-backdrop-filter:none!important;}',
      });
      const sharp = await sample(await page.screenshot());
      // Near the border, the rail can overlap the menu's glass. Compare the
      // same layered pixels with only the rail's own blur switched off.
      expect(blurred.contrast).toBeLessThan(sharp.contrast / 2);
      expect(blurred.neighbors).toEqual(sharp.neighbors);
    });
  });
}

for (const width of [1280, 390]) {
  test(`composer menu scrollbar stays outside its visible border at width ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 400 });
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
    const container = (await page.locator(".composer-wrapper").boundingBox())!;
    const button = (await trigger.boundingBox())!;
    expect(container.y - (frame.y + frame.height)).toBeCloseTo(
      button.y - container.y,
      1,
    );
    const bar = (await handle.boundingBox())!;
    expect(bar.x).toBeGreaterThan(frame.x + frame.width);
    expect(bar.x + bar.width).toBeLessThan(width);
    const footer = popup.locator('[data-menu-fixed="footer"]');
    const fixed = await footer.boundingBox();
    await page.mouse.move(bar.x + bar.width / 2, bar.y + bar.height / 2);
    await expect(handle).toHaveCSS("opacity", "1");
    const expanded = (await handle.boundingBox())!;
    expect(expanded.x - frame.x - frame.width).toBeCloseTo(1, 1);
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
