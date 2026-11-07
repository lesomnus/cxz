import { expect, test } from "@playwright/test";

const stories = [
  "pages-projects--list",
  "pages-projects--default",
  "pages-memory--default",
  "pages-memory--document",
  "pages-search--default",
  "resources-tasks--project-table",
  "resources-tasks--task-details",
  "accounts-workflows--signed-in-account",
  "workspace-errors--overview",
];
for (const theme of ["dark", "light"])
  for (const story of stories)
    test(`${story} inherits the common page surface in ${theme} mode`, async ({
      page,
    }) => {
      await page.goto(
        `/iframe.html?id=${story}&viewMode=story&globals=theme:${theme}`,
      );
      const main = page.locator("[data-resource-page]").first();
      await expect(main).toBeVisible();
      const surface =
        theme === "dark" ? "rgb(20, 20, 20)" : "rgb(250, 250, 250)";
      await expect(main).toHaveCSS("background-color", surface);
      const backgrounds = () =>
        main.evaluate((main) =>
          [...main.querySelectorAll<HTMLElement>('[data-scroll-area="root"]')]
            .filter(
              (area) =>
                !area.parentElement?.closest(
                  '[data-scroll-area="root"], [data-section-layout="section"]',
                ),
            )
            .map((area) => getComputedStyle(area).backgroundColor),
        );
      expect((await backgrounds()).every((value) => value === surface)).toBe(
        true,
      );
      // A semantic token override must reach the page and outer scroll areas without page-specific CSS.
      await main.evaluate((main) =>
        (main as HTMLElement).style.setProperty(
          "--resource-page-background",
          "rgb(9, 10, 11)",
        ),
      );
      await expect(main).toHaveCSS("background-color", "rgb(9, 10, 11)");
      expect(
        (await backgrounds()).every((value) => value === "rgb(9, 10, 11)"),
      ).toBe(true);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
      ).toBe(1280);
    });

test("nested section scroll areas retain their own surface", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--task-details&viewMode=story",
  );
  const main = page.locator("[data-resource-page]");
  const metadata = main
    .getByRole("region", { name: "Task details", exact: true })
    .getByLabel("Task metadata", { exact: true });
  await expect(metadata).toBeVisible();
  const initial = await metadata.evaluate((area) => ({
    background: getComputedStyle(area).backgroundColor,
    section: getComputedStyle(area.closest('[data-section-layout="section"]')!)
      .backgroundColor,
  }));
  expect(initial.background).toBe(initial.section);
  await main.evaluate((main) =>
    (main as HTMLElement).style.setProperty(
      "--resource-page-background",
      "rgb(9, 10, 11)",
    ),
  );
  await expect(metadata).toHaveCSS("background-color", initial.section);
});
