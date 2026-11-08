import { expect, type Page } from "@playwright/test";

export async function pickSearchDate(
  page: Page,
  label: "From (UTC)" | "Until (UTC, exclusive)",
  date: string,
) {
  const field = label === "From (UTC)" ? "since" : "until";
  const current =
    (await page.locator(`input[name="${field}"]`).inputValue()) ||
    (await page.evaluate(() => new Date().toISOString().slice(0, 10)));
  const monthIndex = (value: string) =>
    Number(value.slice(0, 4)) * 12 + Number(value.slice(5, 7));
  const months = monthIndex(date) - monthIndex(current);
  await page.getByRole("button", { name: `${label}:`, exact: false }).click();
  const popup = page.getByRole("dialog", {
    name: `Calendar for ${label}`,
    exact: true,
  });
  for (let i = 0; i < Math.abs(months); i++) {
    await popup
      .getByRole("button", {
        name: months > 0 ? "Next month" : "Previous month",
        exact: true,
      })
      .click();
  }
  await popup
    .locator(`[data-sliding-page][data-active] [data-date="${date}"]`)
    .click();
  await expect(popup).toHaveCount(0);
}
