import { expect, test, type Page } from "@playwright/test";

const table = (page: Page) =>
  page.getByRole("table", { name: "Sessions", exact: true });
const rows = (page: Page) => table(page).locator("tbody tr");
const states = (page: Page) =>
  rows(page).locator(".session-indicator[role=img]");
const stateLabels = (page: Page) =>
  states(page).evaluateAll((nodes) =>
    nodes.map((node) => node.getAttribute("aria-label")),
  );
async function openBulk(page: Page, action: string) {
  await page
    .getByRole("button", { name: "Selected session actions", exact: true })
    .click();
  if (action === "Erase")
    await expect(
      page.getByRole("menuitem", { name: "Erase", exact: true }),
    ).toHaveCSS(
      "font-weight",
      await page
        .getByRole("menuitem", { name: "Restart", exact: true })
        .evaluate((node) => getComputedStyle(node).fontWeight),
    );
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
  const account = (await rows(page)
    .first()
    .getByRole("link", { name: "Work account work", exact: true })
    .boundingBox())!;
  const accountCell = (await rows(page)
    .first()
    .locator("td")
    .nth(2)
    .boundingBox())!;
  expect(button.width).toBeCloseTo(button.height, 1);
  expect(button.y).toBe(account.y);
  expect(button.height).toBe(account.height);
  expect(button.x).toBeGreaterThanOrEqual(cell.x);
  expect(button.x + button.width).toBeLessThanOrEqual(cell.x + cell.width);
  await more.click();
  await expect(page.getByRole("menuitem")).toHaveText([
    "Stop",
    "Resume",
    "Restart",
    "Erase",
  ]);
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("menuitem", { name: "Resume", exact: true }),
  ).toBeDisabled();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const stop = page.getByRole("dialog", { name: "Stop session", exact: true });
  await expect(stop).toContainText("Plan the next release");
  await stop.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(states(page).first()).toHaveAccessibleName("Awaiting answer");
  await openRow(page, "Stop");
  await stop.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(stop).toHaveCount(0);
  await expect.poll(() => stateLabels(page)).toEqual(["Stopped", "Idle"]);
  await more.click();
  await expect(page.getByRole("menuitem")).toHaveText([
    "Stop",
    "Resume",
    "Restart",
    "Erase",
  ]);
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("menuitem", { name: "Resume", exact: true }),
  ).toBeEnabled();
  await page.keyboard.press("Escape");
  await openRow(page, "Resume");
  await expect.poll(() => stateLabels(page)).toEqual(["Idle", "Idle"]);
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
  await expect(bulk).toBeEnabled();
  await bulk.click();
  await expect(page.getByRole("menu").getByRole("status")).toHaveText(
    "0 selected",
  );
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  const before = await table(page).boundingBox();
  const buttonBefore = await bulk.boundingBox();
  await expect(bulk).toHaveAttribute("aria-label", "Selected session actions");
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  expect(await table(page).boundingBox()).toEqual(before);
  expect(await bulk.boundingBox()).toEqual(buttonBefore);
  await expect(bulk).toHaveAttribute("aria-label", "Selected session actions");
  await openBulk(page, "Stop");
  const stop = page.getByRole("dialog", { name: "Stop sessions", exact: true });
  await expect(stop).toContainText("2 sessions will be affected.");
  await stop.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(stop).toHaveCount(0);
  await expect.poll(() => stateLabels(page)).toEqual(["Stopped", "Stopped"]);
  await expect(bulk).toBeEnabled();
  await rows(page).first().getByRole("checkbox").check();
  await openBulk(page, "Resume");
  await expect.poll(() => stateLabels(page)).toEqual(["Idle", "Stopped"]);
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
    page.getByRole("button", { name: "Selected session actions", exact: true }),
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

test("erase confirmation can be canceled and only erases selected targets", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  await openRow(page, "Erase");
  const single = page.getByRole("dialog", {
    name: "Erase session",
    exact: true,
  });
  await expect(single).toContainText("Plan the next release");
  await expect(single).toContainText("This cannot be undone.");
  await single.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(rows(page)).toHaveCount(2);
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  await openBulk(page, "Erase");
  const all = page.getByRole("dialog", { name: "Erase sessions", exact: true });
  await expect(all).toContainText("Review changes");
  await all.getByRole("button", { name: "Erase", exact: true }).click();
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
  await expect.poll(() => stateLabels(page)).toEqual(["Stopped", "Idle"]);
  await expect(rows(page).first().getByRole("checkbox")).not.toBeChecked();
  await expect(rows(page).nth(1).getByRole("checkbox")).toBeChecked();
  await expect(
    page.getByRole("button", { name: "Selected session actions", exact: true }),
  ).toBeVisible();
});

test("failed erase preserves its target and selection for retry", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--erase-failure&viewMode=story",
  );
  await table(page)
    .getByRole("checkbox", { name: "Select all sessions", exact: true })
    .check();
  await openBulk(page, "Erase");
  const dialog = page.getByRole("dialog", {
    name: /^Erase sessions?$/,
    exact: true,
  });
  await expect(rows(page)).toHaveCount(2);
  await dialog.getByRole("button", { name: "Erase", exact: true }).click();
  await expect(dialog).toContainText("Simulated session action failure");
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first().getByRole("checkbox")).toBeChecked();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
});

test("stacked session and account links preserve selected-row emphasis and cell spacing", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const row = rows(page).first();
  const links = row.getByRole("link");
  const before = await table(page).boundingBox();
  await expect(links).toHaveCount(2);
  for (const link of await links.all()) {
    const spacing = await link.evaluate((node) => {
      const cell = node.closest("td")!.getBoundingClientRect();
      const button = node.getBoundingClientRect();
      return {
        top: button.top - cell.top,
        bottom: cell.bottom - button.bottom,
      };
    });
    expect(spacing.top).toBeCloseTo(spacing.bottom, 1);
    await expect(link).toHaveCSS("border-top-width", "0px");
    await expect(link).toHaveCSS("box-shadow", "none");
  }
  await row.getByRole("checkbox").check();
  await row.hover({ position: { x: 16, y: 20 } });
  await expect
    .poll(() =>
      row.evaluate((node) => {
        const shade = (element: Element) =>
          Number(getComputedStyle(element).backgroundColor.match(/\d+/)?.[0]);
        return [...node.querySelectorAll("a")].every(
          (link) => shade(link) > shade(node),
        );
      }),
    )
    .toBe(true);
  await links.evaluateAll((nodes) =>
    Promise.all(
      nodes.flatMap((node) =>
        node
          .getAnimations()
          .map((animation) => animation.finished.catch(() => {})),
      ),
    ),
  );
  const colors = () =>
    row.evaluate((node) =>
      [node, ...node.querySelectorAll("a")].map(
        (element) => getComputedStyle(element).backgroundColor,
      ),
    );
  const selected = await colors();
  await page.mouse.move(0, 0);
  await expect.poll(colors).toEqual(selected);
  await expect(row.getByRole("checkbox")).toBeChecked();
  expect(await table(page).boundingBox()).toEqual(before);
  await expect(row.locator(".editable-value-trigger")).toHaveCount(0);
});

test("unavailable runtime keeps cached session and account navigation available", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  const row = rows(page).first();
  const session = row.getByRole("link", {
    name: "Plan the next release oak",
    exact: true,
  });
  const account = row.getByRole("link", {
    name: "Work account work",
    exact: true,
  });
  await expect(session).toHaveAttribute("href", "/c/_/session/oak");
  await expect(account).toHaveAttribute("href", "/c/_/account/work");
  await row.getByRole("checkbox").check();
  const before = await table(page).boundingBox();
  await session.focus();
  await expect(session).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(account).toBeFocused();
  await expect(row.getByRole("checkbox")).toBeChecked();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await table(page).boundingBox()).toEqual(before);
});

test("the header Add session button opens the existing create form for the current project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const section = page.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const add = section.getByRole("button", { name: "Add session", exact: true });
  const bulk = section.getByRole("button", {
    name: "Selected session actions",
    exact: true,
  });
  await expect(bulk).toBeEnabled();
  await expect(add).toBeEnabled();
  const addBox = (await add.boundingBox())!;
  const bulkBox = (await bulk.boundingBox())!;
  expect(addBox.x + addBox.width).toBeLessThanOrEqual(bulkBox.x);
  expect(addBox.y).toBe(bulkBox.y);
  expect(addBox.width).toBe(bulkBox.width);
  const header = section.getByRole("columnheader", {
    name: "Controls",
    exact: true,
  });
  await expect(
    header.getByRole("button", { name: "Add session", exact: true }),
  ).toBeVisible();
  await expect(section.getByRole("heading")).toHaveCount(0);
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
    rows(page)
      .last()
      .getByRole("link", { name: /^Created from projects/ }),
  ).toContainText("Created from projects");
});
