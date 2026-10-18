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
    await expect(tooltip).toContainText(/left (\d{2}d\d{2}h\d{2}m|--d--h--m)/);
    await expect(tooltip).toHaveCSS("opacity", "1");
    await expect(tooltip).toHaveCSS("transform", "none");
    await expect(tooltip).toHaveCSS("filter", "none");
    await expect(tooltip).toHaveAttribute("data-enter-effect", "reveal-frame");
    const content = tooltip.locator('[data-enter-effect="reveal"]');
    await expect(content).toHaveAttribute("data-enter-from", "from-right");
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
    const center = before.y + before.height / 2;
    for (const item of await content.locator(":scope > *").all()) {
      const bounds = (await item.boundingBox())!;
      expect(Math.abs(bounds.y + bounds.height / 2 - center)).toBeLessThan(1);
    }
    expect(Math.abs(box.y + box.height / 2 - center)).toBeLessThan(1);

    expect(box.x + box.width).toBeLessThan(donut.x);
    expect(box.x + box.width - before.x - before.width).toBeCloseTo(10, 0);
    expect(box.x).toBeLessThan(before.x);
    expect(box.x + box.width).toBeGreaterThan(before.x + before.width);
    expect(box.y).toBeLessThan(before.y);
    expect(box.y + box.height).toBeGreaterThan(before.y + before.height);
    expect(await graph.boundingBox()).toEqual(before);
    if (width !== undefined) expect(box.width).toBeCloseTo(width, 0);
    width = box.width;
    await expect(tooltip.locator(".agent-brand")).toHaveAttribute(
      "aria-label",
      story === "low" ? "Claude" : "Codex",
    );
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

test("duration uses dim padding and intermediate units in nine fixed cells", async ({
  page,
}) => {
  let width: number | undefined;
  for (const story of ["days", "minutes", "zero", "unknown", "overflow"]) {
    await page.goto(
      `/iframe.html?id=components-durationdisplay--${story}&viewMode=story`,
    );
    const value = page.getByRole("img");
    await expect(value).toBeVisible();
    const text = await value.textContent();
    expect(text).toHaveLength(9);
    if (story === "minutes" || story === "zero") {
      const dayColors = await value.evaluate((el) =>
        Array.from(el.children[0].children).map(
          (child) => getComputedStyle(child).color,
        ),
      );
      expect(new Set(dayColors).size).toBe(1);
    }
    if (story === "minutes") {
      expect(text).toBe("00d00h05m");
      const colors = await value.evaluate((el) => {
        const field = el.children[2];
        return Array.from(field.children).map(
          (child) => getComputedStyle(child).color,
        );
      });
      expect(new Set(colors).size).toBe(3);
    }
    const box = (await value.boundingBox())!;
    if (width !== undefined) expect(box.width).toBeCloseTo(width, 0);
    width = box.width;
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
