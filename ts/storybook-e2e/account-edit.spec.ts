import { expect, test } from "@playwright/test";

for (const width of [1280, 390]) {
  test(`account name and alias editing preserves layout and session selection at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
    const work = page.locator('[data-account="work"]');
    const details = work.getByRole("region", {
      name: "Account details",
      exact: true,
    });
    const row = work
      .getByRole("table", { name: "Sessions", exact: true })
      .locator("tbody tr")
      .first();
    await row.getByRole("checkbox").check();
    const original = await details.boundingBox();
    await details
      .getByRole("button", { name: "Edit Display name", exact: true })
      .click();
    const name = page.getByRole("dialog", {
      name: "Edit Display name",
      exact: true,
    });
    await name.getByRole("textbox").fill("Renamed account");
    await name.getByRole("textbox").press("Enter");
    await expect(name).toHaveCount(0);
    await expect(
      details.getByRole("button", { name: "Edit Display name", exact: true }),
    ).toHaveText("Renamed account");
    await expect(row.getByRole("checkbox")).toBeChecked();
    expect(await details.boundingBox()).toEqual(original);
    await details
      .getByRole("button", { name: "Edit Alias", exact: true })
      .click();
    const alias = page.getByRole("dialog", { name: "Edit Alias", exact: true });
    await alias.getByRole("textbox").fill("renamed");
    await alias.getByRole("textbox").press("Enter");
    const renamed = page.locator('[data-account="renamed"]');
    await expect(alias).toHaveCount(0);
    await expect(
      renamed.getByRole("button", { name: "Edit Alias", exact: true }),
    ).toHaveText("renamed");
    await expect(
      renamed.getByRole("button", { name: "Edit Display name", exact: true }),
    ).toHaveText("Renamed account");
    await expect(renamed.locator("tbody tr")).toHaveCount(2);
    await expect(
      renamed.locator("tbody tr").first().getByRole("checkbox"),
    ).toBeChecked();
    await expect(
      renamed.getByRole("region", { name: "Authentication", exact: true }),
    ).toContainText("Signed in");
    expect(
      await renamed
        .getByRole("region", { name: "Account details", exact: true })
        .boundingBox(),
    ).toEqual(original);
    await renamed
      .getByRole("button", { name: "Edit Alias", exact: true })
      .click();
    await alias.getByRole("textbox").fill("discarded");
    await alias.getByRole("textbox").press("Escape");
    await expect(
      renamed.getByRole("button", { name: "Edit Alias", exact: true }),
    ).toHaveText("renamed");
    const backend = renamed.getByText("brokered-access-token", { exact: true });
    await backend.hover();
    await expect(page.getByRole("tooltip")).toHaveCount(0);
  });
}

test("account edits retain drafts after validation and save failures", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-views--account-edit-failure&viewMode=story",
  );
  const work = page.locator('[data-account="work"]');
  await work
    .getByRole("button", { name: "Edit Display name", exact: true })
    .click();
  const name = page.getByRole("dialog", {
    name: "Edit Display name",
    exact: true,
  });
  await name.getByRole("textbox").fill(" ");
  await name.getByRole("textbox").press("Enter");
  await expect(name.getByRole("alert")).toContainText("nonempty");
  await name.getByRole("textbox").fill("Retry name");
  await name.getByRole("textbox").press("Enter");
  await expect(name.getByRole("alert")).toContainText("Account save failed");
  await expect(name.getByRole("textbox")).toHaveValue("Retry name");
  await name.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(name).toHaveCount(0);
  await expect(
    work.getByRole("button", { name: "Edit Display name", exact: true }),
  ).toHaveText("Retry name");
  await work.getByRole("button", { name: "Edit Alias", exact: true }).click();
  const alias = page.getByRole("dialog", { name: "Edit Alias", exact: true });
  await alias.getByRole("textbox").fill("Bad_Alias");
  await alias.getByRole("textbox").press("Enter");
  await expect(alias.getByRole("alert")).toContainText("lowercase");
  await alias.getByRole("textbox").fill("personal");
  await alias.getByRole("textbox").press("Enter");
  await expect(alias.getByRole("alert")).toContainText("already in use");
  await expect(alias.getByRole("textbox")).toHaveValue("personal");
});
