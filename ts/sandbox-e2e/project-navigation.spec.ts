import { expect, test, devices, type Page } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

const picker = (page: Page) =>
  page.getByRole("combobox", { name: "Select project", exact: true });
const visit = (page: Page, name: string) =>
  page.getByRole("link", { name, exact: true }).click();

async function choose(page: Page, name: string) {
  await picker(page).click();
  await page.getByRole("option", { name, exact: true }).click();
  await expect(picker(page)).toHaveText(name);
}

test("the viewed session carries its project into other pages and picks up later project changes", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-8");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  await visit(page, "Projects view");
  await expect(picker(page)).toHaveText("Secondary project");
  await expect(page).toHaveURL(/#\/c\/_\/project\/project-2$/);
  await visit(page, "Tasks");
  await expect(picker(page)).toHaveText("Secondary project");
  await expect(page).toHaveURL(/#\/c\/_\/task\?project=project-2$/);

  await visit(page, "Accounts");
  await page.evaluate(() => (location.hash = "/setting/general"));
  await expect(page).toHaveURL(/#\/setting\/general$/);
  await visit(page, "Projects view");
  await expect(picker(page)).toHaveText("Secondary project");
  await visit(page, "Tasks");
  await choose(page, "Design playground");
  await visit(page, "Projects view");
  await expect(picker(page)).toHaveText("Design playground");

  // Returning to the existing conversation makes its project current again.
  await visit(page, "Sessions view");
  await expect(page).toHaveURL(/#\/c\/_\/session\/session-8$/);
  await visit(page, "Tasks");
  await expect(picker(page)).toHaveText("Secondary project");
  await visit(page, "Memory");
  await expect(page).toHaveURL(/#\/c\/_\/memory\?project=project-2$/);
  await expect(picker(page)).toHaveText("Secondary project");
  await expect(
    page.getByRole("main", { name: "Shared memory workspace", exact: true }),
  ).toBeVisible();
});

test("explicit routes, browser history and reload retain the displayed project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-2");
  await expect(picker(page)).toHaveText("Secondary project", {
    timeout: 45000,
  });
  await choose(page, "Design playground");
  await page.goBack();
  await expect(picker(page)).toHaveText("Secondary project");
  await page.goForward();
  await expect(picker(page)).toHaveText("Design playground");
  await visit(page, "Tasks");
  await expect(picker(page)).toHaveText("Design playground");
  await choose(page, "Secondary project");
  await page.reload();
  await expect(picker(page)).toHaveText("Secondary project", {
    timeout: 45000,
  });
  await visit(page, "Projects view");
  await expect(picker(page)).toHaveText("Secondary project");

  // The unscoped project index does not change the remembered project.
  await page.evaluate(() => (location.hash = "/c/_/project"));
  await expect(
    page.getByRole("table", { name: "Projects", exact: true }),
  ).toBeVisible();
  await expect(picker(page)).toHaveText("Select project");
  await visit(page, "Tasks");
  await expect(picker(page)).toHaveText("Secondary project");
});

test("remembered scope does not override an explicit missing project or guess an unscoped task's project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-2");
  await expect(picker(page)).toHaveText("Secondary project", {
    timeout: 45000,
  });
  await page.evaluate(() => (location.hash = "/c/_/task/missing-task"));
  await expect(
    page.getByText("Choose a project to open this task.", { exact: true }),
  ).toBeVisible();
  await visit(page, "Projects view");
  await expect(picker(page)).toHaveText("Secondary project");
  await page.evaluate(
    () => (location.hash = "/c/_/task?project=missing-project"),
  );
  await expect(
    page.getByText("Project not found. Choose another project.", {
      exact: true,
    }),
  ).toBeVisible();
});

test("reselecting Projects opens the project table and keeps list URLs and history distinct from details", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project/project-2");
  await expect(picker(page)).toHaveText("Secondary project", {
    timeout: 45000,
  });
  await visit(page, "Projects view");
  await expect(page).toHaveURL(/#\/c\/_\/project$/);
  const table = page.getByRole("table", { name: "Projects", exact: true });
  await expect(table).toBeVisible();
  await expect(table.getByRole("columnheader")).toHaveText([
    "",
    "Name",
    "Recent session",
    "Last activity",
    "",
    "",
  ]);
  await expect(table.locator("tbody tr")).toHaveCount(2);
  await expect(picker(page)).toHaveText("Select project");
  await expect(
    table.locator("tbody tr").first().locator("td").nth(2).getByRole("link"),
  ).toBeVisible();
  await page.goBack();
  await expect(picker(page)).toHaveText("Secondary project");
  await page.goForward();
  await expect(table).toBeVisible();
  await page.reload();
  await expect(table).toBeVisible({ timeout: 45000 });
  await expect(page).toHaveURL(/#\/c\/_\/project$/);
  await table
    .getByRole("link", { name: "Secondary project project-2", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/c\/_\/project\/project-2$/);
  await expect(picker(page)).toHaveText("Secondary project");
});

test("reselecting Sessions opens its existing list without forgetting the last conversation or draft", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/session/session-8");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await input.fill("Keep my draft while browsing the session list");
  await visit(page, "Sessions view");
  await expect(page).toHaveURL(/#\/c\/_\/session$/);
  await expect(
    page.getByRole("heading", { name: "Sessions", exact: true }),
  ).toBeVisible();
  const list = page.getByRole("main");
  await expect(list.getByRole("link", { name: /session-8/ })).toHaveAttribute(
    "href",
    "/sandbox.html#/c/_/session/session-8",
  );
  await visit(page, "Sessions view");
  await expect(page).toHaveURL(/#\/c\/_\/session$/);
  await visit(page, "Projects view");
  await expect(picker(page)).toHaveText("Secondary project");
  await visit(page, "Sessions view");
  await expect(page).toHaveURL(/#\/c\/_\/session\/session-8$/);
  await expect(input).toHaveValue(
    "Keep my draft while browsing the session list",
  );
  await visit(page, "Sessions view");
  await list.getByRole("link", { name: /session-8/ }).click();
  await expect(input).toHaveValue(
    "Keep my draft while browsing the session list",
  );
});

test("the project table scrolls inside its viewport on narrow screens", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 700 });
  await page.goto("/sandbox.html#/c/_/project");
  const table = page.getByRole("table", { name: "Projects", exact: true });
  await expect(table).toBeVisible({ timeout: 45000 });
  await expect(
    table.getByRole("link", {
      name: "Design playground project-1",
      exact: true,
    }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  const viewport = page.locator(
    "main > .panel-scroll-region > .panel-scroll-content",
  );
  expect(await viewport.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(
    true,
  );
  await viewport.evaluate((el) => {
    el.scrollLeft = 200;
  });
  expect(await viewport.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
  await page.reload();
  await expect(table).toBeVisible({ timeout: 45000 });
});

test("project row menus confirm lifecycle actions and affect only their project", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/c/_/project");
  const table = page.getByRole("table", { name: "Projects", exact: true });
  const rows = table.locator("tbody tr");
  await expect(rows).toHaveCount(2, { timeout: 45000 });
  const first = rows.first();
  const second = rows.last();
  const more = first.getByRole("button", {
    name: "Controls for Design playground",
    exact: true,
  });
  await first.getByRole("checkbox").check();
  await more.click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  const stop = page.getByRole("dialog", { name: "Stop project", exact: true });
  await stop.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(stop).toHaveCount(0);
  await expect(
    first.getByRole("img", { name: "Running", exact: true }),
  ).toBeVisible();
  const confirm = async (action: string, title: string, button = action) => {
    await more.click();
    await page.getByRole("menuitem", { name: action, exact: true }).click();
    const dialog = page.getByRole("dialog", { name: title, exact: true });
    await expect(dialog).toContainText("Design playground");
    await dialog.getByRole("button", { name: button, exact: true }).click();
    await expect(dialog).toHaveCount(0);
  };
  await confirm("Stop", "Stop project");
  await expect(
    first.getByRole("img", { name: "Stopped", exact: true }),
  ).toBeVisible();
  await expect(first.getByRole("checkbox")).toBeChecked();
  await expect(
    second.getByRole("img", { name: "Running", exact: true }),
  ).toBeVisible();
  await confirm("Start", "Start project");
  await expect(
    first.getByRole("img", { name: "Running", exact: true }),
  ).toBeVisible();
  await confirm("Remove", "Remove container");
  await expect(rows).toHaveCount(2);
  await expect(
    first.getByRole("img", { name: "No container", exact: true }),
  ).toBeVisible();
  await more.click();
  await expect(
    page.getByRole("menuitem", { name: "Stop", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("menuitem", { name: "Remove", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await confirm("Start", "Start project");
  await expect(
    first.getByRole("img", { name: "Running", exact: true }),
  ).toBeVisible();
  await confirm("Erase", "Erase project", "Erase permanently");
  await expect(rows).toHaveCount(1);
  await expect(
    rows.getByRole("link", {
      name: "Secondary project project-2",
      exact: true,
    }),
  ).toBeVisible();
  await expect(page).toHaveURL(/#\/c\/_\/project$/);
});
