import { expect, test } from "@playwright/test";

test("account selection shows identity, scoped sessions, usage and actual login readiness", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const panel = page.getByRole("complementary", { name: "Account list" });
  await expect(
    panel.getByRole("img", { name: "Codex", exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByRole("button", { name: /Work.*work/ }),
  ).toHaveAttribute("aria-pressed", "true");
  const sessions = page.getByRole("region", {
    name: "Sessions using this account",
  });
  await expect(sessions).toContainText("Web account layout");
  await expect(sessions).not.toContainText("Claude project");
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Log out", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByText("Conversation usage", { exact: true }),
  ).toBeVisible();
  await panel.getByRole("button", { name: /Personal.*personal/ }).click();
  await expect(
    page.getByText("Session-scoped login", { exact: true }),
  ).toBeVisible();
  await expect(sessions).toContainText("Claude project");
  await expect(sessions).not.toContainText("Web account layout");
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toHaveCount(0);
});

test("Add opens beside the panel and its language/provider dropdown works inside the modal", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const panel = page.getByRole("complementary", { name: "Account list" });
  await panel.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog).toBeVisible();
  const panelBox = (await panel.boundingBox())!,
    dialogBox = (await dialog.boundingBox())!;
  expect(dialogBox.x).toBeGreaterThanOrEqual(panelBox.x + panelBox.width - 1);
  await dialog.getByRole("combobox", { name: "Agent", exact: true }).click();
  await dialog.getByRole("option", { name: "Claude", exact: true }).click();
  await expect(
    dialog.getByRole("combobox", { name: "Agent", exact: true }),
  ).toContainText("Claude");
  await dialog
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("new-account");
  await dialog
    .getByRole("textbox", { name: "Display name", exact: true })
    .fill("New team account");
  await dialog.getByRole("button", { name: "Add", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    panel.getByRole("button", { name: /New team account.*new-account/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("region", { name: "Account details" }),
  ).toContainText("New team account");
  expect(errors).toEqual([]);
});

test("logout needs confirmation and then enables login", async ({ page }) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Log out · Work",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  await dialog.getByRole("button", { name: "Log out", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Log out", exact: true }),
  ).toBeDisabled();
});
