import { expect, type Page } from "@playwright/test";

export async function expectAPIError(page: Page, message: string) {
  const trigger = page.getByRole("button", {
    name: "Errors",
    exact: true,
    includeHidden: true,
  });
  await expect(trigger).toHaveAttribute("data-active", "true");
  if (!(await trigger.isVisible())) {
    await expect(
      page.getByRole("alert").filter({ hasText: message }),
    ).toBeVisible();
    return;
  }
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await expect(dialog).toContainText(message);
  await dialog
    .getByRole("button", { name: "Close errors", exact: true })
    .click();
}
