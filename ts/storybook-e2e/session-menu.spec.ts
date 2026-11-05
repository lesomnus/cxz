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

for (const height of [801, 400])
  test(`Erase is last in the scrolling menu, above fixed editor options at height ${height}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1000, height });
    await page.goto(
      "/iframe.html?id=conversation-session-menu--always-open&viewMode=story",
    );
    const menu = page.getByRole("menu", { name: "Session menu", exact: true });
    const erase = menu.getByRole("menuitem", { name: "Erase", exact: true });
    await expect(erase).toBeEnabled();
    await expect(menu.getByRole("menuitem").last()).toHaveText("Erase");
    const popup = page.locator(".action-menu-popup");
    const footer = popup.locator('[data-menu-fixed="footer"]');
    await expect(
      footer.getByRole("button", { name: "Word wrap", exact: true }),
    ).toBeVisible();
    expect(
      await erase.evaluate(
        (node) =>
          !!node.closest("[data-menu-body]") &&
          !node.closest("[data-menu-fixed]"),
      ),
    ).toBe(true);
    await erase.scrollIntoViewIfNeeded();
    const er = await erase.boundingBox(),
      foot = await footer.boundingBox();
    expect(er!.y + er!.height).toBeLessThanOrEqual(foot!.y + 1);
    await erase.click();
    const dialog = page.getByRole("dialog", {
      name: "Erase session",
      exact: true,
    });
    await expect(dialog).toContainText("This cannot be undone.");
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByLabel("Last action", { exact: true })).toHaveText(
      "",
    );
    await erase.click();
    await dialog.getByRole("button", { name: "Erase", exact: true }).click();
    await expect(page.getByLabel("Last action", { exact: true })).toHaveText(
      "erase",
    );
    await expect(
      page.getByRole("button", { name: "Session menu", exact: true }),
    ).toBeDisabled();
  });
