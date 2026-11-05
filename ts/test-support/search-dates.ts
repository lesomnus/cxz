import { expect, type Page } from "@playwright/test";

export async function pickSearchDate(
  page: Page,
  label: "From (UTC)" | "Until (UTC, exclusive)",
  date: string,
) {
  await page.getByRole("button", { name: `${label}:`, exact: false }).click();
  const popup = page.getByRole("dialog", {
    name: `Calendar for ${label}`,
    exact: true,
  });
  await popup
    .locator(`[data-sliding-page][data-active] [data-date="${date}"]`)
    .click();
  await expect(popup).toHaveCount(0);
}
