import { expect, test } from "@playwright/test";

for (const theme of ["dark", "light"]) {
  test(`session menu retains integer dimensions through resizing in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1000, height: 801 });
    await page.goto(
      `/iframe.html?id=conversation-session-menu--always-open&viewMode=story&globals=theme:${theme}`,
    );
    const menu = page.getByRole("group", {
      name: "Session menu",
      exact: true,
    });
    await expect(menu).toBeVisible();
    await menu.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => {})),
      );
    });
    for (const viewport of [
      { width: 1000, height: 801 },
      { width: 200, height: 401 },
      { width: 1000, height: 801 },
    ]) {
      await page.setViewportSize(viewport);
      await expect(menu).toBeVisible();
      await expect
        .poll(async () => {
          const width = (await menu.boundingBox())!.width;
          return viewport.width === 200 ? width < 198 : width === 198;
        })
        .toBe(true);
      const fractions = await menu.evaluate((root) =>
        [root, ...root.querySelectorAll("*")]
          .filter((node) => node instanceof HTMLElement)
          .flatMap((node) => {
            const { width, height } = node.getBoundingClientRect();
            return [width, height].every(Number.isInteger)
              ? []
              : [
                  {
                    element: node.tagName,
                    label: node.getAttribute("aria-label") ?? node.className,
                    width,
                    height,
                  },
                ];
          }),
      );
      expect(fractions).toEqual([]);
      const actions = menu.locator('[data-action-menu-layout="row"] > button');
      const widths = await actions.evaluateAll((nodes) =>
        nodes.map((node) => node.getBoundingClientRect().width),
      );
      expect(new Set(widths).size).toBe(1);
      if (viewport.width === 1000) expect(widths).toEqual([64, 64, 64]);
      const rows = menu.locator(
        '.action-menu-group:not([data-action-menu-layout="row"]) > button',
      );
      const heights = await rows.evaluateAll((nodes) =>
        nodes.map((node) => node.getBoundingClientRect().height),
      );
      expect(heights.every((height) => height === 32)).toBe(true);
    }
  });
}
