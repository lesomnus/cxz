import { expect, test } from "@playwright/test";

const story = "/iframe.html?id=workspace-errors--recovery&viewMode=story";
test("centralizes failures, preserves recovery history and keeps canceled retries active", async ({
  page,
}) => {
  await page.goto(story);
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  const profile = page.getByRole("button", { name: "Profile", exact: true });
  const action = (name: string) =>
    page.getByRole("button", { name, exact: true });
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await action("Fail request").click();
  await expect(trigger).toHaveAttribute("data-active", "true");
  await expect(page.getByRole("alert")).toHaveText(
    "Example local input validation",
  );
  const icon = trigger.locator("svg");
  await expect(icon).toHaveCSS("color", "rgb(255, 82, 109)");
  const triggerBox = (await trigger.boundingBox())!;
  expect(triggerBox.y + triggerBox.height).toBeLessThanOrEqual(
    (await profile.boundingBox())!.y,
  );
  await trigger.click();
  await expect(dialog).toContainText("Account service is offline");
  await expect(dialog).toContainText("Account: work");
  await expect(
    dialog.getByRole("button", { name: "Close errors" }),
  ).toBeFocused();
  expect((await dialog.boundingBox())!.x).toBeGreaterThan(
    (await trigger.boundingBox())!.x,
  );
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await action("Fail request").click();
  await action("Succeed for another account").click();
  await expect(trigger).toHaveAttribute("data-active", "true");
  await action("Start retry").click();
  await expect(action("Cancel retry")).toBeEnabled();
  await expect(trigger).toHaveAttribute("data-active", "true");
  await action("Cancel retry").click();
  await expect(action("Start retry")).toBeEnabled();
  await expect(trigger).toHaveAttribute("data-active", "true");
  await action("Recover request").click();
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  await expect(dialog).toContainText("No active errors");
  await dialog.getByRole("radio", { name: "History (1)", exact: true }).click();
  await expect(dialog).toContainText("Resolved");
  await expect(dialog).toContainText("2 occurrences");
  await page.reload();
  await trigger.click();
  await dialog.getByRole("radio", { name: "History (1)", exact: true }).click();
  await expect(dialog).toContainText("Account service is offline");
  await expect(dialog).toContainText("Resolved");
});

test("retains dismissed errors separately, keeps active errors when clearing history and recovers streams", async ({
  page,
}) => {
  await page.goto(story);
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  const action = (name: string) =>
    page.getByRole("button", { name, exact: true });
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await action("Fail request").click();
  await trigger.click();
  await dialog.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(trigger).not.toHaveAttribute("data-active");
  await dialog.getByRole("radio", { name: "History (1)", exact: true }).click();
  await expect(dialog).toContainText("Dismissed");
  await page.keyboard.press("Escape");
  await action("Fail stream").click();
  await expect(trigger).toHaveAttribute("data-active", "true");
  await trigger.click();
  await dialog
    .getByRole("button", { name: "Clear history", exact: true })
    .click();
  await dialog.getByRole("radio", { name: "Active (1)", exact: true }).click();
  await expect(dialog).toContainText("Conversation stream disconnected");
  await expect(
    dialog.getByRole("radio", { name: "History (0)", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await action("Recover stream").click();
  await expect(trigger).not.toHaveAttribute("data-active");
  await trigger.click();
  await dialog.getByRole("radio", { name: "History (1)", exact: true }).click();
  await expect(dialog).toContainText("Resolved");
  await expect(dialog).toContainText("Session: seal");
});

test("captures sign-out failures and keeps the panel within a small viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 740 });
  await page.goto(story);
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
  const trigger = page.getByRole("button", { name: "Errors", exact: true });
  await expect(trigger).toHaveAttribute("data-active", "true");
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await expect(dialog).toContainText("Sign out is unavailable");
  const box = (await dialog.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(8);
  expect(box.x + box.width).toBeLessThanOrEqual(382);
});

test("keeps editable field validation local and reports save API failures only in the error center", async ({
  page,
}) => {
  await page.goto(story);
  await page
    .getByRole("button", { name: "Edit Account alias", exact: true })
    .click();
  const edit = page.getByRole("dialog", {
    name: "Edit Account alias",
    exact: true,
  });
  const input = edit.getByRole("textbox", {
    name: "Account alias",
    exact: true,
  });
  await input.fill("");
  await edit.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(edit).toContainText("Alias is required");
  await input.fill("another");
  await edit.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(edit).not.toHaveAttribute("aria-busy", "true");
  await expect(edit).not.toContainText("Account service is offline");
  await expect(input).toHaveValue("another");
  await edit.getByRole("button", { name: "Cancel", exact: true }).click();
  const errors = page.getByRole("button", { name: "Errors", exact: true });
  await expect(errors).toHaveAttribute("data-active", "true");
  await errors.click();
  await expect(
    page.getByRole("dialog", { name: "Errors", exact: true }),
  ).toContainText("Account service is offline");
});
