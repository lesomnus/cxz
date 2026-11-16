import { expect, test } from "@playwright/test";

for (const theme of ["dark", "light"]) {
  test(`session Resume shares project Start colors in ${theme}`, async ({
    page,
  }) => {
    const colors = (node: HTMLElement) => {
      const css = getComputedStyle(node);
      return { color: css.color, background: css.backgroundColor };
    };
    await page.goto(
      `/iframe.html?id=pages-projects--stopped&viewMode=story&globals=theme:${theme}`,
    );
    const start = page.locator('[data-project-action="start"]');
    await expect(start).toBeEnabled();
    const resting = await start.evaluate(colors);
    await start.hover();
    await expect
      .poll(() => start.evaluate((node) => node.getAnimations().length))
      .toBe(0);
    const hovered = await start.evaluate(colors);
    await page.goto(
      `/iframe.html?id=conversation-session-menu--stopped&viewMode=story&globals=theme:${theme}`,
    );
    const resume = page.getByRole("menuitem", { name: "Resume", exact: true });
    await expect(resume).toBeEnabled();
    await expect.poll(() => resume.evaluate(colors)).toEqual(resting);
    await resume.hover();
    await expect.poll(() => resume.evaluate(colors)).toEqual(hovered);
    await resume.click();
    await page
      .getByRole("button", { name: "Session menu", exact: true })
      .click();
    await expect(
      page.getByRole("menuitem", { name: "Resume", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("menuitem", { name: "Stop", exact: true }),
    ).toBeEnabled();
  });
}
