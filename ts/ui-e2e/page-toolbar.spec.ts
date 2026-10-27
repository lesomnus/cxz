import { expect, test } from "@playwright/test";

test("portable page toolbar switches the selected project", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=layout-pagetoolbar--project-selector&viewMode=story",
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
});
