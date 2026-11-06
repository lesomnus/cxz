import { expect, test } from "@playwright/test";

for (const width of [1000, 892, 891, 700, 360]) {
  test(`account and project dashboards share their layout at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    const layouts = [];
    for (const story of [
      "resources-views--accounts",
      "pages-projects--default",
    ]) {
      await page.goto(`/iframe.html?id=${story}&viewMode=story`);
      const dashboard = page.locator("[data-resource-dashboard]").first();
      await expect(dashboard).toBeVisible();
      await dashboard.evaluate((node, width) => {
        const main = node.closest("main")!;
        main.style.flex = "none";
        main.style.width = `${width}px`;
      }, width);
      await expect
        .poll(() =>
          dashboard.evaluate((node) => node.closest("main")!.clientWidth),
        )
        .toBe(width);
      const layout = await dashboard.evaluate((node) => {
        const primary = node.querySelector(
          '[data-resource-dashboard-column="information"]',
        )!;
        const usage = node.querySelector('[aria-label="Conversation usage"]')!;
        const sessions = primary
          .querySelector('[aria-label="Session list"]')!
          .closest("section")!;
        const left = primary.getBoundingClientRect(),
          right = usage.getBoundingClientRect();
        const css = getComputedStyle(node),
          inset = getComputedStyle(sessions);
        return {
          columnWidth: left.width,
          usageWidth: right.width,
          primaryHeight: left.height,
          twoColumns: right.x > left.x,
          width: node.getBoundingClientRect().width,
          gap: css.columnGap,
          canonicalWidth: parseFloat(
            css.getPropertyValue("--resource-page-column-width"),
          ),
          gutter: parseFloat(css.getPropertyValue("--resource-page-gutter")),
          innerRadius: getComputedStyle(primary.firstElementChild!)
            .borderTopRightRadius,
          sessionsInset: [
            inset.paddingTop,
            inset.paddingLeft,
            inset.paddingRight,
          ],
        };
      });
      expect(layout.twoColumns).toBe(width >= 892);
      expect(layout.columnWidth).toBe(
        Math.min(layout.canonicalWidth, width - 2 * layout.gutter),
      );
      expect(layout.usageWidth).toBe(layout.columnWidth);
      layouts.push(layout);
    }
    expect(layouts[0]).toEqual(layouts[1]);
  });
}

test("one resource width token resizes both dashboards and their toolbars", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  for (const story of [
    "resources-views--accounts",
    "pages-projects--default",
  ]) {
    await page.goto(`/iframe.html?id=${story}&viewMode=story`);
    const dashboard = page.locator("[data-resource-dashboard]").first();
    await expect(dashboard).toBeVisible();
    await dashboard.evaluate((node) => {
      const main = node.closest("main")!;
      main.style.flex = "none";
      main.style.width = "1000px";
      main.style.setProperty("--resource-page-column-width", "384px");
      main.style.setProperty("--resource-page-column-gap", "8px");
    });
    await expect
      .poll(() =>
        dashboard.evaluate((node) => node.getBoundingClientRect().width),
      )
      .toBe(776);
    const widths = await dashboard.evaluate((node) => ({
      information: node
        .querySelector('[data-resource-dashboard-column="information"]')!
        .getBoundingClientRect().width,
      usage: node
        .querySelector('[aria-label="Conversation usage"]')!
        .getBoundingClientRect().width,
      toolbar: node
        .closest("main")!
        .querySelector("header > div > div")!
        .getBoundingClientRect().width,
    }));
    expect(widths).toEqual({ information: 384, usage: 384, toolbar: 776 });
  }
});
