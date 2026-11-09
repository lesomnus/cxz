import { expect, type Page } from "@playwright/test";

/** Settings topics scroll within one page, using a picker on compact layouts. */
export async function chooseSettingsTopic(
  page: Page,
  name: string,
  label = "Settings topics",
) {
  const fileToggle = page.locator(
    'button[aria-controls="settings-file-editor"][aria-pressed="true"]',
  );
  if (await fileToggle.isVisible()) await fileToggle.click();
  const picker = page.getByRole("combobox", {
    name: label,
    exact: true,
  });
  if (await picker.isVisible()) {
    await picker.click();
    await page.getByRole("option", { name, exact: true }).click();
    await expect(picker).toHaveText(name);
  } else {
    const topic = page
      .getByRole("navigation", { name: label, exact: true })
      .getByRole("button", { name, exact: true });
    await topic.click();
    await expect(topic).toHaveAttribute("aria-current", "location");
  }
}
