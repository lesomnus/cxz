import { expect, test } from "@playwright/test";

const story = (name: string) =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
const detail = (page: import("@playwright/test").Page) =>
  page.getByRole("region", { name: "Task details", exact: true });

test("random assignment uses a live project session and preserves edits and the new task version", async ({
  page,
}) => {
  await page.goto(story("single-task-details"));
  const title = detail(page).getByRole("textbox", {
    name: "Title",
    exact: true,
  });
  await expect(title).toHaveValue("Implement Task MCP tools");
  await title.fill("Keep my draft during assignment");
  const action = detail(page).getByRole("button", {
    name: "Assign randomly",
    exact: true,
  });
  await expect(action.locator('[data-icon="dice"]')).toBeVisible();
  await action.click();
  await expect(action).toBeDisabled();
  await expect(
    detail(page).getByLabel("Status", { exact: true }),
  ).toContainText("Working");
  const group = detail(page).getByRole("group", {
    name: "Assigned session controls",
    exact: true,
  });
  await expect(group.getByRole("link")).toHaveAttribute(
    "href",
    "/c/_/session/oak",
  );
  const inactive = group.getByRole("button", {
    name: "No assignment action",
    exact: true,
  });
  await expect(inactive).toBeDisabled();
  await expect(inactive.locator('[data-icon="dot"]')).toBeVisible();
  await expect(title).toHaveValue("Keep my draft during assignment");
  const save = detail(page).getByRole("button", { name: "Save", exact: true });
  await expect(save).toBeEnabled();
  await save.click();
  await expect(save).toBeDisabled();
  await expect(title).toHaveValue("Keep my draft during assignment");
  await expect(page.getByLabel("Review requests sent")).toHaveText("0");
});

test("previous-run review requires confirmation and sends once without discarding edits", async ({
  page,
}) => {
  await page.goto(story("assignment-actions"));
  const title = detail(page).getByRole("textbox", {
    name: "Title",
    exact: true,
  });
  await expect(title).toHaveValue("Restore interrupted work");
  await title.fill("Unsaved review draft");
  const action = detail(page).getByRole("button", {
    name: "Request task review",
    exact: true,
  });
  await expect(action.locator('[data-icon="send-message"]')).toBeVisible();
  await action.click();
  const modal = page.getByRole("dialog", {
    name: "Request task review?",
    exact: true,
  });
  await expect(modal).toContainText("Task API (oak)");
  await expect(modal).toContainText(
    'Please review task "Restore interrupted work"',
  );
  await expect(modal).toContainText("00000000-0000-0000-0000-000000000010");
  await expect(page.getByLabel("Review requests sent")).toHaveText("0");
  await modal.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(modal).toHaveCount(0);
  await expect(page.getByLabel("Review requests sent")).toHaveText("0");
  await action.click();
  const confirm = modal.getByRole("button", { name: "Send", exact: true });
  await confirm.click();
  await expect(confirm).toBeDisabled();
  await expect(modal).toHaveCount(0);
  await expect(page.getByLabel("Review requests sent")).toHaveText("1");
  await expect(
    detail(page).getByText("Task review request queued for oak.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(title).toHaveValue("Unsaved review draft");
  await expect(
    detail(page).getByRole("button", { name: "Save", exact: true }),
  ).toBeEnabled();
  await expect(
    detail(page)
      .locator("dt")
      .filter({ hasText: /^Assignment$/ }),
  ).toHaveCount(0);
});
