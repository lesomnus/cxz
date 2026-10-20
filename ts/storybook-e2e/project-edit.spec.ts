import { expect, test, type Page } from "@playwright/test";

const information = (page: Page) =>
  page.getByRole("region", { name: "Information", exact: true });
async function edit(page: Page, label: string, value: string) {
  await information(page)
    .getByRole("button", { name: `Edit ${label}`, exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: `Edit ${label}`,
    exact: true,
  });
  await dialog.getByRole("textbox", { name: label, exact: true }).fill(value);
  return dialog;
}

test("project edits preserve identity, selection and layout", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const section = information(page);
  const row = page
    .getByRole("table", { name: "Sessions", exact: true })
    .locator("tbody tr")
    .first();
  await row.getByRole("checkbox").check();
  const before = await section.boundingBox();
  const trigger = section.getByRole("button", {
    name: "Edit Name",
    exact: true,
  });
  const box = (await trigger.boundingBox())!;
  const name = await edit(page, "Name", "Renamed workspace");
  const inputBox = (await name.getByRole("textbox").boundingBox())!;
  expect(inputBox.x).toBeCloseTo(box.x, 0);
  expect(inputBox.y).toBeCloseTo(box.y, 0);
  expect(await section.boundingBox()).toEqual(before);
  await name.getByRole("textbox").press("Enter");
  await expect(name).toHaveCount(0);
  await expect(trigger).toHaveText("Renamed workspace");
  await expect(
    page.getByRole("combobox", { name: "Select project", exact: true }),
  ).toHaveText("Renamed workspace");
  const alias = await edit(page, "Alias", "  New-Alias  ");
  await alias.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(alias).toHaveCount(0);
  await expect(
    section.getByRole("button", { name: "Edit Alias", exact: true }),
  ).toHaveText("new-alias");
  const desc = await edit(page, "Description", "");
  await desc.getByRole("textbox").press("Enter");
  await expect(desc).toHaveCount(0);
  await expect(
    section.getByRole("button", { name: "Edit Description", exact: true }),
  ).toHaveText("—");
  await expect(trigger).toHaveText("Renamed workspace");
  await expect(row.getByRole("checkbox")).toBeChecked();
  expect(await section.boundingBox()).toEqual(before);
  const cancel = await edit(page, "Name", "Discarded");
  await cancel.getByRole("textbox").press("Escape");
  await expect(trigger).toHaveText("Renamed workspace");
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(
    section.getByRole("button", { name: "Edit Name", exact: true }),
  ).toHaveText("API workspace");
});

test("duplicate aliases and invalid names retain drafts", async ({ page }) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const alias = await edit(page, "Alias", "api");
  await alias.getByRole("textbox").press("Enter");
  await expect(alias.getByRole("alert")).toContainText("already in use");
  await expect(alias.getByRole("textbox")).toHaveValue("api");
  await alias.getByRole("textbox").press("Escape");
  const name = await edit(page, "Name", " ");
  await name.getByRole("textbox").press("Enter");
  await expect(name.getByRole("alert")).toContainText("nonempty");
  await name.getByRole("textbox").press("Escape");
  await expect(
    information(page).getByRole("button", { name: "Edit Name", exact: true }),
  ).toHaveText("Web workspace");
});

test("failed saves keep the draft and original metadata", async ({ page }) => {
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  const dialog = await edit(page, "Description", "Keep this draft");
  await dialog.getByRole("textbox").press("Enter");
  await expect(dialog.getByRole("alert")).toContainText(
    "Project is unavailable",
  );
  await expect(dialog.getByRole("textbox")).toHaveValue("Keep this draft");
  await dialog.getByRole("textbox").press("Escape");
  await expect(
    information(page).getByRole("button", {
      name: "Edit Description",
      exact: true,
    }),
  ).toHaveText("Frontend and design system");
});
