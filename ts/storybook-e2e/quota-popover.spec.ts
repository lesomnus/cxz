import { expect, test } from "@playwright/test";

test("quota hover opens above the stationary outlined graph with stable columns", async ({
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
    await expect(tooltip).toContainText(/left (\d{2}d\d{2}h\d{2}m|--d--h--m)/);
    await expect(tooltip).toHaveCSS("opacity", "1");
    await expect(tooltip).toHaveCSS("transform", "none");
    await expect(tooltip).toHaveCSS("filter", "none");
    await expect(tooltip).toHaveAttribute("data-enter-effect", "reveal-frame");
    const content = tooltip.locator('[data-enter-effect="reveal"]');
    await expect(content).toHaveAttribute(
      "data-enter-from",
      "from-bottom-right",
    );
    await expect(content.locator(":scope > span").last()).toHaveText(
      story === "full"
        ? "100%"
        : story === "low"
          ? "1%"
          : story === "weekly"
            ? "72%"
            : "—",
    );
    await expect(content).toHaveCSS("animation-duration", "0.36s");
    await expect(content).toHaveCSS("filter", "blur(0px)");
    await expect(tooltip).toHaveCSS("font-size", "10px");
    await expect(tooltip).toHaveCSS("border-top-width", "0px");
    if (story === "weekly") await expect(tooltip).toContainText("weekly");
    const glyphs = await graph.locator("span").boundingBox();
    const donut = (await page.locator(".context-donut").boundingBox())!;
    expect(glyphs!.x + glyphs!.width).toBeLessThan(donut.x);
    const box = (await tooltip.boundingBox())!;
    const center = box.y + box.height / 2;
    for (const item of await content.locator(":scope > *").all()) {
      const bounds = (await item.boundingBox())!;
      expect(Math.abs(bounds.y + bounds.height / 2 - center)).toBeLessThan(1);
    }
    expect(Math.abs(box.y + box.height / 2 - center)).toBeLessThan(1);

    expect(box.x + box.width).toBeLessThan(donut.x);
    expect(box.x + box.width - before.x - before.width).toBeCloseTo(4, 0);
    expect(box.x).toBeLessThan(before.x);
    expect(box.x + box.width).toBeGreaterThan(before.x + before.width);
    expect(box.y).toBeLessThan(before.y);
    expect(box.y + box.height).toBeLessThan(before.y);
    await expect(graph.locator("..")).toHaveCSS("outline-width", "1px");
    expect(await graph.boundingBox()).toEqual(before);
    if (width !== undefined) expect(box.width).toBeCloseTo(width, 0);
    width = box.width;
    await expect(tooltip.locator(".agent-brand")).toHaveCount(0);
    await expect(tooltip).toHaveCSS("backdrop-filter", "blur(12px)");
    await page.mouse.move(box.x + 15, box.y + box.height / 2);
    await expect
      .poll(() => tooltip.evaluate((node) => node.matches(":hover")))
      .toBe(true);
    await expect(tooltip).toBeVisible();
    await page.mouse.move(16, 16);
    await expect
      .poll(() =>
        graph.locator("..").evaluate((node) => node.matches(":hover")),
      )
      .toBe(false);
    await expect(tooltip).toBeHidden();
    await graph.locator("..").focus();
    await expect(tooltip).toBeVisible();
    await graph.locator("..").blur();
    await expect(tooltip).toBeHidden();
  }
});

test("quota reveal respects reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=conversation-usage--full&viewMode=story");
  await page.locator(".quota-dots").hover();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toHaveCSS("animation-name", "none");
  await expect(tooltip.locator('[data-enter-effect="reveal"]')).toHaveCSS(
    "animation-name",
    "none",
  );
});

test("quota panel keeps a 2px graph gap as composer height changes", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-usage--inside-composer&viewMode=story",
  );
  const graph = page.locator(".quota-dots");
  const input = page.locator(".composer-input");
  await graph.locator("..").focus();
  const panel = page.getByRole("tooltip");
  await expect(panel).toBeVisible();
  await expect(panel).toHaveCSS("opacity", "1");
  for (const height of [120, 240]) {
    await input.evaluate((el, height) => {
      (el as HTMLElement).style.height = height + "px";
    }, height);
    await expect
      .poll(async () => {
        const trigger = (await graph.locator("..").boundingBox())!;
        const popup = (await panel.boundingBox())!;
        return Math.abs(trigger.y - popup.y - popup.height - 2);
      })
      .toBeLessThan(1);
  }
  await graph.locator("..").blur();
  await graph.hover();
  await expect(panel).toHaveCSS("opacity", "1");
  const trigger = (await graph.locator("..").boundingBox())!;
  const popup = (await panel.boundingBox())!;
  await page.mouse.move(trigger.x + trigger.width / 2, trigger.y - 1);
  await expect(panel).toBeVisible();
  await page.mouse.move(
    trigger.x + trigger.width / 2,
    popup.y + popup.height / 2,
  );
  await expect(panel).toBeVisible();
});
