import { expect, test, type Page } from "@playwright/test";

const table = (page: Page) =>
  page.getByRole("table", { name: "Sessions", exact: true });
const rows = (page: Page) => table(page).locator("tbody tr");
const states = (page: Page) => rows(page).locator("td:nth-child(6)");
async function openBulk(page: Page, action: string) {
  await page
    .getByRole("button", { name: "Selected session actions", exact: true })
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
  expect(button.width).toBeCloseTo(button.height, 1);
  expect(button.x).toBe(cell.x);
  expect(button.y).toBe(cell.y);
  expect(button.width).toBe(cell.width);
  expect(Math.abs(button.height - cell.height)).toBeLessThanOrEqual(1);
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
    name: "Selected session actions",
    exact: true,
  });
  await expect(bulk).toBeDisabled();
  const before = await table(page).boundingBox();
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  expect(await table(page).boundingBox()).toEqual(before);
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
    page.getByRole("status", { name: "3 selected", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Selected session actions", exact: true })
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
    page.getByRole("status", { name: "1 selected", exact: true }),
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
