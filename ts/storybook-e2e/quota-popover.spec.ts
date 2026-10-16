import { expect, test } from "@playwright/test";

test("quota hover opens left, covers the stationary graph and has stable columns", async ({
  page,
}) => {
  let width: number | undefined;
  for (const story of ["full", "low", "weekly", "unreported"]) {
    await page.goto(
      `/iframe.html?id=conversation-usage--${story}&viewMode=story`,
    );
    const graph = page.locator(".quota-dots");
    await expect(graph).toBeVisible();
    const before = (await graph.boundingBox())!;
    await graph.hover();
    const tooltip = page.getByRole("tooltip");
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toContainText(
      /left (\d+d \d{2}h \d{2}m|--d --h --m)/,
    );
    await expect(tooltip).toHaveCSS("opacity", "1");
    await expect(tooltip).toHaveCSS("clip-path", "inset(0px round 6px)");
    await expect(tooltip).toHaveCSS("font-size", "10px");
    await expect(tooltip).toHaveCSS("border-top-width", "0px");
    if (story === "weekly") await expect(tooltip).toContainText("weekly");
    const glyphs = await graph.locator("span").boundingBox();
    const donut = (await page.locator(".context-donut").boundingBox())!;
    expect(glyphs!.x + glyphs!.width).toBeLessThan(donut.x);
    const box = (await tooltip.boundingBox())!;
    expect(box.x + box.width).toBeLessThan(donut.x);
    expect(box.x + box.width - before.x - before.width).toBeCloseTo(10, 0);
    expect(box.x).toBeLessThan(before.x);
    expect(box.x + box.width).toBeGreaterThan(before.x + before.width);
    expect(box.y).toBeLessThan(before.y);
    expect(box.y + box.height).toBeGreaterThan(before.y + before.height);
    expect(await graph.boundingBox()).toEqual(before);
    if (width !== undefined) expect(box.width).toBeCloseTo(width, 0);
    width = box.width;
    await expect(tooltip.getByRole("img")).toHaveAttribute(
      "aria-label",
      story === "low" ? "Claude" : "Codex",
    );
    await page.mouse.move(box.x + 15, box.y + box.height / 2);
    await expect(tooltip).toBeVisible();
    await page.mouse.move(0, 0);
    await expect(tooltip).toBeHidden();
    await graph.locator("..").focus();
    await expect(tooltip).toBeVisible();
  }
});
