import { expect, test } from "@playwright/test";

test("bottom-left Erase action requires confirmation and leaves other projects intact", async ({
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
  const trigger = actions.getByRole("button", { name: "Erase", exact: true });
  const descriptionBox = (await description.boundingBox())!;
  const buttonBox = (await trigger.boundingBox())!;
  expect(buttonBox.x).toBeGreaterThan(descriptionBox.x + descriptionBox.width);
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Erase project",
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
    .getByRole("button", { name: "Erase permanently", exact: true })
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
    .getByRole("button", { name: "Erase", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Erase permanently", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByText("No projects yet.", { exact: true }),
  ).toBeVisible();
});

test("project erase failure retains the confirmation and selected project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Erase", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Erase project",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Erase permanently", exact: true })
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

test("Stop confirms, stops all project sessions and retains the project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const actions = page.getByRole("region", { name: "Actions", exact: true });
  const stop = actions.getByRole("button", { name: "Stop", exact: true });
  const rows = page
    .getByRole("table", { name: "Sessions", exact: true })
    .locator("tbody tr");
  await rows.first().getByRole("checkbox").check();
  await stop.click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await expect(dialog).toContainText(
    "The project remains listed and can be started again.",
  );
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(stop).toBeEnabled();
  await expect(stop).toBeFocused();
  await stop.click();
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(stop).toBeDisabled();
  await expect(rows).toHaveCount(2);
  await expect(rows.locator("td:nth-child(6)")).toHaveText([
    "Stopped",
    "Stopped",
  ]);
  await expect(rows.first().getByRole("checkbox")).toBeChecked();
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Web workspace");
  await expect(
    actions.getByRole("button", { name: "Erase", exact: true }),
  ).toBeEnabled();
  await picker.click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(
    actions.getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await expect(rows.locator("td:nth-child(6)")).toHaveText([
    "Waiting for input",
    "Idle",
  ]);
  await picker.click();
  await page
    .getByRole("option", { name: "Web workspace", exact: true })
    .click();
  await expect(
    actions.getByRole("button", { name: "Stop", exact: true }),
  ).toBeDisabled();
});

test("failed project Stop retains the confirmation and records", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await page
    .getByRole("region", { name: "Actions", exact: true })
    .getByRole("button", { name: "Stop", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Stop project",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Project stop is unavailable",
  );
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    page
      .getByRole("region", { name: "Actions", exact: true })
      .getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await expect(
    page
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
});
