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
  await expect(
    page.getByRole("combobox", { name: "Session", exact: true }),
  ).toContainText("session-8");
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

  // A bare list URL still displays its documented default project.
  await page.evaluate(() => (location.hash = "/c/_/project"));
  await expect(picker(page)).toHaveText("Design playground");
  await visit(page, "Tasks");
  await expect(picker(page)).toHaveText("Design playground");
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
