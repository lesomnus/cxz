import { expect, type Page } from "@playwright/test";

export async function chooseSetting(page: Page, label: string, value: string) {
  const control = page
    .getByLabel(label, { exact: true })
    .and(page.locator('input, select, [role="combobox"], [role="radiogroup"]'));
  if (await control.evaluate((el) => el.matches("select"))) {
    await control.selectOption(value);
  } else if (
    await control.evaluate((el) => el.matches('input[type="range"]'))
  ) {
    await control.press("Home");
    for (let index = 0; index < Number(value || 0); index++)
      await control.press("ArrowRight");
  } else if ((await control.getAttribute("role")) === "radiogroup") {
    await control.locator(`input[value="${value}"]`).check();
  } else {
    await control.click();
    await page
      .getByRole("listbox")
      .locator(`[data-option-value="${value}"]`)
      .click();
  }
}
export async function expectInherited(page: Page, label: string) {
  const control = page
    .getByLabel(label, { exact: true })
    .and(page.locator('input, select, [role="combobox"], [role="radiogroup"]'));
  if (await control.evaluate((el) => el.matches('input[type="range"]')))
    await expect(control).toHaveValue("0");
  else if ((await control.getAttribute("role")) === "radiogroup")
    await expect(control).toHaveAttribute("data-value", "");
  else await expect(control.locator("..")).toHaveAttribute("data-value", "");
}
