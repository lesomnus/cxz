import { expect, test } from "@playwright/test";

test("hover account usage opens to the right without stealing editor focus and reuses cached reads", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-sidebar--default&viewMode=story");
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await expect(trigger).toBeVisible({ timeout: 20000 });
  await expect(page.getByLabel("Account preview reads")).toHaveText("0");
  const editor = page.getByRole("textbox", { name: "Conversation input" });
  await editor.fill("Draft stays focused");
  await trigger.hover();
  const menu = page.getByRole("dialog", { name: "Account usage" });
  await expect(menu).toBeVisible();
  await expect(editor).toBeFocused();
  await expect(menu.locator(".agent-brand-codex")).toBeVisible();
  await expect(menu.locator(".agent-brand-claude")).toBeVisible();
  await expect(menu.getByLabel("64% remaining")).toBeVisible();
  await expect(menu.getByLabel("Remaining usage unavailable")).toBeVisible();
  const buttonBox = (await trigger.boundingBox())!;
  const box = (await menu.boundingBox())!;
  expect(box.x).toBeGreaterThan(buttonBox.x + buttonBox.width);
  await expect(menu.getByRole("link")).toHaveCount(0);
  await expect(menu.getByRole("button")).toHaveCount(0);
  expect(box.width).toBeLessThan(300);
  await expect(menu).toHaveCSS("border-top-width", "0px");
  await menu.getByRole("listitem").filter({ hasText: "Work" }).click();
  await expect(menu).toBeVisible();
  await page.mouse.move(box.x + 20, box.y + 15, { steps: 12 });
  await expect(menu).toBeVisible();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  const count = await page.getByLabel("Account preview reads").textContent();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await page.mouse.move(500, 350);
  await trigger.hover();
  await expect(menu.getByLabel("64% remaining")).toBeVisible();
  await expect(page.getByLabel("Account preview reads")).toHaveText(count!);
  await page.mouse.move(500, 350);
  await expect(menu).toHaveCount(0);
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
});

test("keyboard users can open and dismiss the account preview, and unknown quota is not zero", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-sidebar--usage-unavailable&viewMode=story",
  );
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await expect(trigger).toBeVisible({ timeout: 20000 });
  await trigger.focus();
  const menu = page.getByRole("dialog", { name: "Account usage" });
  await expect(menu.getByLabel("Remaining usage unavailable")).toHaveCount(2);
  await expect(trigger).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("an account-free connection shows an empty state", async ({ page }) => {
  await page.goto("/iframe.html?id=accounts-sidebar--empty&viewMode=story");
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await expect(trigger).toBeVisible({ timeout: 20000 });
  await trigger.hover();
  await expect(
    page.getByRole("dialog", { name: "Account usage" }),
  ).toContainText("No accounts");
});
