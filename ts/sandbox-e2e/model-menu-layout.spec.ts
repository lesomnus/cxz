import { expect, test, type Locator } from "@playwright/test";

async function textBounds(value: Locator) {
  return value.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const rect = range.getBoundingClientRect();
    return { x: rect.x, y: rect.y };
  });
}

for (const width of [1440, 390]) {
  test(`model and effort overlays preserve the text position at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/sandbox.html");
    await expect(
      page.getByRole("textbox", { name: "Message", exact: true }),
    ).toBeVisible({ timeout: 45000 });

    for (const direction of ["up", "down"]) {
      if (direction === "down") {
        // A denser host trigger need not fill ValueMenu's wrapper. The
        // overlay must follow its text, including when it opens downward.
        await page.addStyleTag({
          content: `
            .composer-meta { position: fixed; top: 40px; left: 24px; }
            .model-info .setting-trigger {
              padding-block: 0; min-height: 20px; height: 20px;
            }
          `,
        });
      }
      for (const name of ["Model", "Effort"]) {
        const trigger = page.getByRole("combobox", { name, exact: true });
        const before = await textBounds(trigger.locator(".meta-value"));
        await trigger.click();
        const menu = page.locator(".setting-menu");
        await expect(menu).toBeVisible();
        const after = await textBounds(
          menu.locator(".setting-current .meta-value"),
        );
        expect(after.x).toBeCloseTo(before.x, 1);
        expect(after.y).toBeCloseTo(before.y, 1);
        const bounds = (await menu.boundingBox())!;
        expect(bounds.x).toBeGreaterThanOrEqual(0);
        expect(bounds.x + bounds.width).toBeLessThanOrEqual(width);
        if (name === "Model") expect(bounds.width).toBeLessThan(200);
        await page.keyboard.press("Escape");
        await expect(menu).toHaveCount(0);
      }
    }
  });
}
