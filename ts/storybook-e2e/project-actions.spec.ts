import { expect, test } from "@playwright/test";

test("bottom-left Purge action requires confirmation and leaves other projects intact", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  await expect(actions).toBeVisible();
  const sections = actions.locator("..").locator(":scope > section");
  await expect(sections.last()).toHaveAttribute("aria-label", "Actions");
  const description = actions.getByText(
    "Permanently delete this project and its owned data.",
    { exact: true },
  );
  const trigger = actions.getByRole("button", { name: "Purge", exact: true });
  const descriptionBox = (await description.boundingBox())!;
  const buttonBox = (await trigger.boundingBox())!;
  expect(buttonBox.x).toBeGreaterThan(descriptionBox.x + descriptionBox.width);
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Purge project",
    exact: true,
  });
  await expect(dialog).toContainText("This cannot be undone.");
  await expect(dialog).toContainText(
    "Workspace source files and shared accounts are preserved.",
  );
  await expect(dialog).toContainText("Web workspace");
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Web workspace");
  await trigger.click();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await trigger.click();
  await dialog
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("API workspace");
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
  await picker.click();
  await expect(
    page.getByRole("option", { name: "Web workspace", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Purge", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByText("No projects yet.", { exact: true }),
  ).toBeVisible();
});

test("project purge failure retains the confirmation and selected project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Purge", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Purge project",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Project removal is unavailable",
  );
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeEnabled();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Web workspace");
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
});
