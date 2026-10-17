import { test, expect } from "@playwright/test";

test("context reveals left with stable numeric slots and a stationary donut", async ({
  page,
}) => {
  let width: number | undefined;
  for (const story of ["full", "small-context", "unreported"]) {
    await page.goto(
      `/iframe.html?id=conversation-usage--${story}&viewMode=story`,
    );
    const donut = page.locator(".context-donut");
    await expect(donut).toBeVisible();
    const before = (await donut.boundingBox())!;
    await donut.hover();
    const popup = page.getByRole("tooltip");
    await expect(popup).toBeVisible();
    await expect(popup).toHaveAttribute("data-enter-effect", "reveal-frame");
    const content = popup.locator('[data-enter-effect="reveal"]');
    await expect(content).toHaveAttribute("data-enter-from", "from-right");
    await expect(content).toHaveCSS("filter", "blur(0px)");
    const box = (await popup.boundingBox())!;
    expect(box.x).toBeLessThan(before.x);
    expect(box.x + box.width).toBeGreaterThan(before.x + before.width);
    expect(await donut.boundingBox()).toEqual(before);
    expect(
      Math.abs(box.y + box.height / 2 - before.y - before.height / 2),
    ).toBeLessThan(1);
    if (width !== undefined) expect(box.width).toBeCloseTo(width, 0);
    width = box.width;
    if (story === "small-context")
      await expect(popup).toHaveText("0123·/0999·12%");
    const percent = content.locator(":scope > span").last();
    await expect(percent).toHaveText(
      story === "full" ? "24%" : story === "small-context" ? "12%" : "—",
    );
    await expect(percent).toHaveCSS("border-left-width", "1px");
    const colors = await content.evaluate((el) => {
      const number = el.children[0];
      const separator = el.children[1];
      return {
        value: getComputedStyle(number.children[1]).color,
        unit: getComputedStyle(number.children[2]).color,
        slash: getComputedStyle(separator).color,
        padding: getComputedStyle(separator).paddingLeft,
      };
    });
    expect(colors.unit).toBe(colors.slash);
    expect(colors.value).not.toBe(colors.unit);
    expect(parseFloat(colors.padding)).toBeGreaterThan(0);
    await page.mouse.move(box.x + 10, box.y + box.height / 2);
    await expect(popup).toBeVisible();
    await page.mouse.move(0, 0);
    await expect(popup).toBeHidden();
    await donut.locator("..").focus();
    await expect(popup).toBeVisible();
  }
});
