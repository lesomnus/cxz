import { test, expect } from "@playwright/test";
test("composer setting hover frames include icons without moving values or showing tooltips", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--interactive&viewMode=story",
  );
  for (const name of ["Model", "Effort"]) {
    const field = page.locator(`.${name.toLowerCase()}-field`);
    const menu = page.getByRole("combobox", { name, exact: true });
    await expect(menu).toBeVisible();
    const before = await menu.boundingBox();
    await expect(field.locator(".meta-tooltip")).toHaveCount(0);
    await menu.hover();
    const frame = await field.evaluate((el) => {
      const s = getComputedStyle(el, "::before");
      return {
        opacity: s.opacity,
        top: s.top,
        left: s.left,
        right: s.right,
        bottom: s.bottom,
        background: s.backgroundColor,
      };
    });
    expect(frame.opacity).toBe("1");
    expect([frame.top, frame.left, frame.right, frame.bottom]).toEqual([
      "-2px",
      "-2px",
      "-2px",
      "-2px",
    ]);
    expect(await menu.boundingBox()).toEqual(before);
    expect(
      await menu.evaluate((el) => getComputedStyle(el).backgroundColor),
    ).not.toBe(frame.background);
    await menu.click();
    await expect(page.getByRole("option").first()).toBeVisible();
    await page.mouse.move(0, 0);
    expect(
      await field.evaluate((el) => getComputedStyle(el, "::before").opacity),
    ).toBe("1");
    await page.keyboard.press("Escape");
  }
});
