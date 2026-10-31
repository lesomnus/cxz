import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 900 },
});

test("session erasure requires confirmation and preserves the owning project and other sessions", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-2");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(1, { timeout: 45000 });
  const more = rows.first().getByRole("button", { name: /^Controls for / });
  const dialog = page.getByRole("dialog", {
    name: "Erase session",
    exact: true,
  });
  await more.click();
  await expect(
    page.getByRole("menuitem", { name: "Purge", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  await expect(dialog).toContainText("This cannot be undone.");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(rows).toHaveCount(1);
  await more.click();
  await page.getByRole("menuitem", { name: "Erase", exact: true }).click();
  await dialog.getByRole("button", { name: "Erase", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(rows).toHaveCount(0);
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toContainText("Secondary project");
  await picker.click();
  await page
    .getByRole("option", { name: "Design playground", exact: true })
    .click();
  await expect(rows).toHaveCount(8);
  await picker.click();
  await page
    .getByRole("option", { name: "Secondary project", exact: true })
    .click();
  await expect(rows).toHaveCount(0);
});
