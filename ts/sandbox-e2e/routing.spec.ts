import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("sandbox shares routes through hash history and keeps scenario selection in sync", async ({
  page,
}) => {
  await page.goto("/sandbox.html#/sessions/session-2");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible({ timeout: 45000 });
  await expect(page.getByLabel("Scenario", { exact: true })).toHaveValue(
    "session-2",
  );
  await input.fill("Sandbox route draft");
  await page.getByRole("link", { name: "Settings view", exact: true }).click();
  await expect(page).toHaveURL(/#\/settings\/general$/);
  await page.goBack();
  await expect(input).toHaveValue("Sandbox route draft");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(page).toHaveURL(/#\/sessions\/session-1$/);
  await page.goBack();
  await expect(page.getByLabel("Scenario", { exact: true })).toHaveValue(
    "session-2",
  );
  await expect(input).toHaveValue("Sandbox route draft");
  await page.reload();
  await expect(input).toBeVisible({ timeout: 45000 });
  await expect(page.getByLabel("Scenario", { exact: true })).toHaveValue(
    "session-2",
  );
  await page.goto("/sandbox.html#/settings/editor");
  await expect(
    page.getByRole("heading", { name: "Editor", exact: true }),
  ).toBeVisible({ timeout: 45000 });
});
