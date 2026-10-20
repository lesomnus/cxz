import { expect, test, type Page } from "@playwright/test";

const table = (page: Page) =>
  page.getByRole("table", { name: "Sessions", exact: true });
const rows = (page: Page) => table(page).locator("tbody tr");
const states = (page: Page) => rows(page).locator("td:nth-child(6)");
async function openBulk(page: Page, action: string) {
  await page
    .getByRole("button", { name: /^\d+ selected$/, exact: true })
    .click();
  await page.getByRole("menuitem", { name: action, exact: true }).click();
}
async function openRow(page: Page, action: string) {
  await rows(page)
    .first()
    .getByRole("button", { name: /Controls for/ })
    .click();
  await page.getByRole("menuitem", { name: action, exact: true }).click();
}

test("row controls confirm stop/restart and resume the intended session", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const more = rows(page)
    .first()
    .getByRole("button", { name: /Controls for/ });
  await expect(more).toBeVisible();
  await expect(more).toBeInViewport();
  const button = (await more.boundingBox())!;
  const cell = (await rows(page).first().locator("td").last().boundingBox())!;
  const account = (await rows(page).first().getByRole("link").boundingBox())!;
  const accountCell = (await rows(page)
    .first()
    .locator("td")
    .nth(4)
    .boundingBox())!;
  expect(button.width).toBeCloseTo(button.height, 1);
  expect(button.y).toBe(account.y);
  expect(button.height).toBe(account.height);
  expect(button.x - cell.x).toBe(account.x - accountCell.x);
  expect(cell.x + cell.width - button.x - button.width).toBe(
    accountCell.x +
      accountCell.width -
      account.x -
      account.width +
      button.x -
      cell.x,
  );
  await openRow(page, "Stop");
  const stop = page.getByRole("dialog", { name: "Stop session", exact: true });
  await expect(stop).toContainText("Plan the next release");
  await stop.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(states(page).first()).toHaveText("Waiting for input");
  await openRow(page, "Stop");
  await stop.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(stop).toHaveCount(0);
  await expect(states(page)).toHaveText(["Stopped", "Idle"]);
  await openRow(page, "Resume");
  await expect(states(page)).toHaveText(["Idle", "Idle"]);
  await openRow(page, "Restart");
  const restart = page.getByRole("dialog", {
    name: "Restart session",
    exact: true,
  });
  await expect(restart).toContainText(
    "does not recreate the project container",
  );
  await restart.getByRole("button", { name: "Restart", exact: true }).click();
  await expect(restart).toHaveCount(0);
  await expect(rows(page)).toHaveCount(2);
});

test("bulk controls keep layout stable and apply only to the selected sessions", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const bulk = page.getByRole("button", {
    name: /^\d+ selected$/,
    exact: true,
  });
  await expect(bulk).toBeDisabled();
  const before = await table(page).boundingBox();
  const buttonBefore = await bulk.boundingBox();
  await expect(bulk).toHaveAttribute("aria-label", "0 selected");
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  expect(await table(page).boundingBox()).toEqual(before);
  expect(await bulk.boundingBox()).toEqual(buttonBefore);
  await expect(bulk).toHaveAttribute("aria-label", "2 selected");
  await openBulk(page, "Stop");
  const stop = page.getByRole("dialog", { name: "Stop sessions", exact: true });
  await expect(stop).toContainText("2 sessions will be affected.");
  await stop.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(stop).toHaveCount(0);
  await expect(states(page)).toHaveText(["Stopped", "Stopped"]);
  await expect(bulk).toBeDisabled();
  await rows(page).first().getByRole("checkbox").check();
  await openBulk(page, "Resume");
  await expect(states(page)).toHaveText(["Idle", "Stopped"]);
  await expect(rows(page).first().getByRole("checkbox")).not.toBeChecked();
});

test("mixed-state bulk stop skips sessions already stopped", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--session-states&viewMode=story",
  );
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  await openBulk(page, "Stop");
  const dialog = page.getByRole("dialog", {
    name: "Stop sessions",
    exact: true,
  });
  await expect(dialog).toContainText("6 sessions will be affected.");
  await expect(dialog).toContainText("already stopped are skipped");
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "3 selected", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: /^\d+ selected$/, exact: true })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("menuitem", { name: "Resume", exact: true }),
  ).toBeEnabled();
});

test("purge previews can be canceled and confirmed batch purge removes only its targets", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  await openRow(page, "Purge");
  const single = page.getByRole("dialog", {
    name: "Purge session",
    exact: true,
  });
  await expect(single).toContainText("/storybook/web-0/journal");
  await expect(single).toContainText("Account settings");
  await single.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(rows(page)).toHaveCount(2);
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  await openBulk(page, "Purge");
  const all = page.getByRole("dialog", { name: "Purge sessions", exact: true });
  await expect(all).toContainText("/storybook/web-1/journal");
  await all
    .getByRole("button", { name: "Purge permanently", exact: true })
    .click();
  await expect(all).toHaveCount(0);
  await expect(rows(page)).toHaveCount(0);
  await expect(page.getByText("No sessions", { exact: true })).toBeVisible();
});

test("partial failures keep only failed targets in the confirmation and selection", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--partial-failure&viewMode=story",
  );
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  await openBulk(page, "Stop");
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toContainText("Some actions failed");
  await expect(dialog).toContainText("Simulated session action failure");
  await expect(dialog).not.toContainText("Plan the next release");
  await expect(dialog).toContainText("Review changes");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(states(page)).toHaveText(["Stopped", "Idle"]);
  await expect(rows(page).first().getByRole("checkbox")).not.toBeChecked();
  await expect(rows(page).nth(1).getByRole("checkbox")).toBeChecked();
  await expect(
    page.getByRole("button", { name: "1 selected", exact: true }),
  ).toBeVisible();
});

test("a failed purge preview prevents all deletion", async ({ page }) => {
  await page.goto(
    "/iframe.html?id=pages-projects--purge-preview-failure&viewMode=story",
  );
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  await openBulk(page, "Purge");
  await expect(page.getByRole("alert")).toContainText(
    "Simulated session action failure",
  );
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(rows(page)).toHaveCount(2);
  await expect(
    table(page).getByRole("checkbox", {
      name: "Select all sessions",
      exact: true,
    }),
  ).toBeChecked();
});

test("session names and aliases edit in place without changing row selection or layout", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const row = rows(page).first();
  await row.getByRole("checkbox").check();
  const before = await table(page).boundingBox();
  const editName = row.getByRole("button", { name: "Edit Name", exact: true });
  const origin = (await editName.boundingBox())!;
  await editName.click();
  const dialog = page.getByRole("dialog", { name: "Edit Name", exact: true });
  const input = dialog.getByRole("textbox", { name: "Name", exact: true });
  await expect(input).toBeFocused();
  const overlay = (await input.boundingBox())!;
  for (const axis of ["x", "y", "width", "height"] as const)
    expect(overlay[axis]).toBeCloseTo(origin[axis], 1);
  await input.fill("Release plan updated");
  await input.press("Enter");
  await expect(dialog).toHaveCount(0);
  await expect(editName).toHaveText("Release plan updated");
  await expect(row.getByRole("checkbox")).toBeChecked();
  expect(await table(page).boundingBox()).toEqual(before);

  const editAlias = row.getByRole("button", {
    name: "Edit Alias",
    exact: true,
  });
  await editAlias.click();
  const aliasDialog = page.getByRole("dialog", {
    name: "Edit Alias",
    exact: true,
  });
  const alias = aliasDialog.getByRole("textbox", {
    name: "Alias",
    exact: true,
  });
  await alias.fill("pine");
  await alias.press("Escape");
  await expect(editAlias).toHaveText("oak");
  await editAlias.click();
  await alias.fill("Bad");
  await alias.press("Enter");
  await expect(aliasDialog.getByRole("alert")).toContainText("Alias must be");
  await alias.fill("elm");
  await alias.press("Enter");
  await expect(aliasDialog.getByRole("alert")).toContainText("already in use");
  await expect(alias).toHaveValue("elm");
  await alias.fill("oak-tree");
  await aliasDialog
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(aliasDialog).toHaveCount(0);
  await expect(editAlias).toHaveText("oak-tree");
  await expect(editName).toHaveText("Release plan updated");
  await expect(row.getByRole("checkbox")).toBeChecked();
  expect(await table(page).boundingBox()).toEqual(before);
});

test("failed identity saves keep the draft and leave the row unchanged", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  const row = rows(page).first();
  const trigger = row.getByRole("button", { name: "Edit Name", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Edit Name", exact: true });
  const input = dialog.getByRole("textbox", { name: "Name", exact: true });
  await input.fill("Keep this draft");
  await input.press("Enter");
  await expect(dialog.getByRole("alert")).toContainText(
    "Session is unavailable",
  );
  await expect(input).toHaveValue("Keep this draft");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trigger).toHaveText("Plan the next release");
});

test("the rightmost Session button opens the existing create form for the current project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const section = page.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const add = section.getByRole("button", { name: "Add session", exact: true });
  const bulk = section.getByRole("button", { name: "0 selected", exact: true });
  await expect(bulk).toBeDisabled();
  await expect(add).toBeEnabled();
  const addBox = (await add.boundingBox())!;
  const bulkBox = (await bulk.boundingBox())!;
  expect(addBox.x).toBeGreaterThan(bulkBox.x + bulkBox.width);
  await expect(
    section.getByRole("button", { name: "Actions", exact: true }),
  ).toHaveCount(0);
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await picker.click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await add.click();
  const dialog = page.getByRole("dialog", { name: "Add session", exact: true });
  await expect(
    dialog.getByRole("combobox", { name: "Project", exact: true }),
  ).toHaveText("API workspace");
  await expect
    .poll(() =>
      dialog.evaluate((node) => node.contains(document.activeElement)),
    )
    .toBe(true);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(add).toBeFocused();
  await picker.click();
  await page
    .getByRole("option", { name: "Web workspace", exact: true })
    .click();
  await add.click();
  await dialog
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Created from projects");
  await dialog.getByRole("combobox", { name: "Account", exact: true }).click();
  await page
    .getByRole("option", { name: "Work account · codex", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Create session", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(rows(page)).toHaveCount(3);
  await expect(
    rows(page).last().getByRole("button", { name: "Edit Name", exact: true }),
  ).toHaveText("Created from projects");
});
