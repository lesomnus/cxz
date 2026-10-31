import { test, expect } from "@playwright/test";
for (const theme of ["dark", "light"]) {
  test(`remaining quota colors follow thresholds in ${theme} mode`, async ({
    page,
  }) => {
    for (const [story, level] of [
      ["full", "normal"],
      ["warning", "warning"],
      ["warning-boundary", "warning"],
      ["low", "critical"],
      ["exhausted", "critical"],
      ["unreported", "unknown"],
    ]) {
      await page.goto(
        `/iframe.html?id=conversation-usage--${story}&viewMode=story&globals=theme:${theme}`,
      );
      const graph = page.locator(".quota-dots");
      await expect(graph.locator("..")).toHaveAttribute(
        "data-quota-level",
        level,
      );
      await graph.hover();
      const popup = page.getByRole("tooltip");
      await expect(popup).toBeVisible();
      const percent = popup.getByRole("cell").last();
      if (level === "warning" || level === "critical") {
        const expected =
          level === "warning"
            ? theme === "dark"
              ? "rgb(157, 130, 69)"
              : "rgb(138, 98, 0)"
            : theme === "dark"
              ? "rgb(176, 72, 91)"
              : "rgb(197, 46, 72)";
        await expect(graph).toHaveCSS("color", expected);
        await expect(percent).toHaveCSS("color", expected);
      }
      expect(
        await page
          .locator(".context-donut")
          .evaluate((el) => getComputedStyle(el).color),
      ).not.toBe("rgb(176, 72, 91)");
    }
  });
}
