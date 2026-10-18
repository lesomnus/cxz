import { expect, test } from "@playwright/test";

test("project page and shared toolbar stories render their selector and states", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=components-pagetoolbar--project-selector&viewMode=story",
  );
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Web workspace");
  await picker.click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(picker).toHaveText("API workspace");
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  await expect(
    page.getByRole("main", { name: "Project workspace" }),
  ).toBeVisible();
  await expect(picker).toHaveText("Web workspace");
  await page
    .getByRole("region", { name: "Project sessions", exact: true })
    .getByRole("button")
    .first()
    .click();
  await expect(
    page.getByRole("region", { name: "Session details", exact: true }),
  ).toBeVisible();
  await expect(picker).toHaveText("Web workspace");
  await page.goto("/iframe.html?id=pages-projects--empty&viewMode=story");
  await expect(picker).toBeDisabled();
  await expect(
    page.getByText("No projects yet.", { exact: true }),
  ).toBeVisible();
  await page.goto("/iframe.html?id=pages-projects--loading&viewMode=story");
  await expect(picker).toBeDisabled();
  await expect(page.locator("main .panel-scroll-content")).toContainText(
    "Loading projects…",
  );
  expect(errors).toEqual([]);
});
