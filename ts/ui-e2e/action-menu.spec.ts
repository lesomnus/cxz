import { expect, test } from "@playwright/test";

test("More and language menus use separate popups with keyboard navigation", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=components-actionmenu--more&viewMode=story");
  const more = page.getByRole("button", { name: "More", exact: true });
  await more.press("ArrowDown");
  await expect(
    page.getByRole("menuitem", { name: "Details", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("End");
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menu")).toHaveCount(0);
  await expect(more).toBeFocused();

  await page.goto(
    "/iframe.html?id=components-actionmenu--language&viewMode=story",
  );
  const language = page.getByRole("button", { name: "Voice language" });
  await expect(language).toHaveText("English (US)");
  await language.click();
  const current = page.getByRole("menuitemcheckbox", {
    name: "English (US)",
    exact: true,
  });
  await expect(current).toHaveAttribute("aria-checked", "true");
  const triggerBox = await language.boundingBox();
  const menuBox = await page.getByRole("menu").boundingBox();
  expect(menuBox!.y).toBeGreaterThan(triggerBox!.y + triggerBox!.height);
  await page
    .getByRole("menuitemcheckbox", { name: "한국어", exact: true })
    .click();
  await expect(language).toHaveText("한국어");
  await expect(page.getByRole("menu")).toHaveCount(0);
  await language.click();
  await expect(
    page.getByRole("menuitemcheckbox", { name: "한국어", exact: true }),
  ).toHaveAttribute("aria-checked", "true");
});
